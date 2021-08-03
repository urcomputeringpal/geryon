package geryon

import (
	"log"
	"net/http"
	"time"

	"github.com/urcomputeringpal/geryon/webhooks"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v25/github"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

// Geryon is a Kubernetes controller that keeps your cluster in sync with your
// GitHub account. At its core, it's a GitHub Client, a Kubernetes client, a
// work queue, and a bunch of workers.
type Geryon struct {
	GithubAppsClient       *github.Client
	KubernetesClient       *kubernetes.Clientset
	NamespaceInformerCache cache.Store
	gitHubApp              *webhooks.Server
	queue                  workqueue.RateLimitingInterface
	Config                 Config
}

const (
	// InstallationIDAnnotation is added to all namespaces created by geryon
	InstallationIDAnnotation = "urcomputeringpal.com/geryon-installation-id"

	// ImagePullSyncAnnotation is used to determine whether or not to sync secrets on a namespace
	ImagePullSyncAnnotation = "urcomputeringpal.com/geryon-sync-image-pull-secrets"

	// ImagePullSyncTimestampAnnotation is added to the secret and is updated when it is synced
	ImagePullSyncTimestampAnnotation = "urcomputeringpal.com/geryon-image-pull-secrets-sync-timestamp"

	// DockerSecretName is the name of the secret containing Docker credentials
	DockerSecretName = "github-package-registry"
)

// Config contains the configuration for a Geryon controller
type Config struct {
	WebhookPort                int
	GitHubAppID                int64
	GitHubAppPrivateKeyFile    string
	GithubAppWebHookSecret     string
	Kubeconfig                 string
	InstallationResyncInterval time.Duration
	NamespaceResyncInterval    time.Duration
	Threadiness                int
}

// NewGeryon creates a Geryon controller
func NewGeryon(config Config) (*Geryon, error) {
	cfg, err := clientcmd.BuildConfigFromFlags("", config.Kubeconfig)
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	appsTransport, err := ghinstallation.NewAppsTransportKeyFromFile(http.DefaultTransport, config.GitHubAppID, config.GitHubAppPrivateKeyFile)
	if err != nil {
		return nil, err
	}
	githubAppsClient := github.NewClient(&http.Client{Transport: appsTransport})

	queue := workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(time.Second*5, time.Minute))

	w := &webhooks.Server{
		Port:            config.WebhookPort,
		WebhookSecret:   config.GithubAppWebHookSecret,
		AppID:           config.GitHubAppID,
		PrivateKeyFile:  config.GitHubAppPrivateKeyFile,
		GitHubAppClient: githubAppsClient,
		Queue:           queue,
	}

	return &Geryon{
		Config:           config,
		GithubAppsClient: githubAppsClient,
		KubernetesClient: kubeClient,
		gitHubApp:        w,
		queue:            queue,
	}, nil
}

// Run takes one stop channel and coordinates all of our various workers
func (g *Geryon) Run(stopCh <-chan struct{}) error {
	// Run the shared namespace informer
	factory := informers.NewSharedInformerFactory(g.KubernetesClient, time.Second*30)
	namespaceInformer := factory.Core().V1().Namespaces().Informer()
	informerStopCh := make(chan struct{})
	go namespaceInformer.Run(informerStopCh)
	if !cache.WaitForCacheSync(informerStopCh, namespaceInformer.HasSynced) {
		log.Fatal("error waiting for informer cache to sync")
	}
	g.NamespaceInformerCache = namespaceInformer.GetStore()

	// Run the webhook listener, and setup a channel for graceful shutdown
	webhookStopChan := make(chan struct{})
	webhookShutdownChan := make(chan struct{})
	go func() {
		if err := g.gitHubApp.Run(webhookStopChan, webhookShutdownChan); err != nil {
			log.Fatal(err)
		}
	}()

	// Resync all existing installations every 5 minutes
	nsStopChan := make(chan struct{})
	go wait.Until(g.ResyncInstallationNamespaces, g.Config.InstallationResyncInterval, nsStopChan)

	// Resync managed Namespaces every 29 minutes
	resyncStopChan := make(chan struct{})
	go wait.Until(g.ResyncManagedNamespaces, g.Config.NamespaceResyncInterval, resyncStopChan)

	// Run the workers
	workerStopChan := make(chan struct{})
	for i := 0; i < g.Config.Threadiness; i++ {
		go wait.Until(g.runWorker, time.Second, workerStopChan)
	}

	log.Printf("Geryon ready\n")

	// Run until we receive a signal
	<-stopCh

	// Immediately stop things that crete work
	close(webhookStopChan)
	close(resyncStopChan)
	close(nsStopChan)

	// Wait until those things stop
	<-webhookShutdownChan

	// Close the workers
	close(workerStopChan)

	log.Printf("Geryon out\n")
	return nil
}
