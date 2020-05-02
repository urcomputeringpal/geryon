package geryon

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v25/github"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SyncNamespace ensures a Namespace exists for each Installation, and that
// it's default ServiceAccount has be configured with imagePullSecrets for the
// GitHub Package Registry
func (g *Geryon) SyncNamespace(nameAndInstallationID string) error {
	parts := strings.Split(nameAndInstallationID, ":")
	name := parts[0]
	installationID := parts[1]
	ns, exists, err := g.NamespaceInformerCache.GetByKey(name)
	if err != nil {
		return err
	}
	if !exists {
		namespaceClient := g.KubernetesClient.CoreV1().Namespaces()
		newNamespace := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Annotations: map[string]string{
					ImagePullSyncAnnotation:  "",
					InstallationIDAnnotation: installationID,
				},
			},
		}
		ns, err = namespaceClient.Create(context.Background(), newNamespace, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		log.Printf("Created %s namespace\n", name)
	}
	namespace := ns.(*v1.Namespace)

	secretClient := g.KubernetesClient.CoreV1().Secrets(namespace.GetName())

	secret, err := secretClient.Get(context.Background(), DockerSecretName, metav1.GetOptions{})
	create := false
	update := false
	if err != nil {
		create = true
		secret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: DockerSecretName,
			},
			Type: "kubernetes.io/dockercfg",
		}
	}
	if metav1.HasAnnotation(secret.ObjectMeta, ImagePullSyncTimestampAnnotation) {
		lastUpdated, err := time.Parse(time.RFC3339, secret.Annotations[ImagePullSyncTimestampAnnotation])
		if err != nil {
			return err
		}
		now := time.Now()
		duration := now.Sub(lastUpdated)
		if duration > (time.Minute * 5) {
			update = true
		}
	}

	if create || update {
		intID, err := strconv.Atoi(installationID)
		if err != nil {
			return err
		}

		token, err := g.getInstallationToken(int64(intID))
		if err != nil {
			return err
		}
		secret.Annotations = map[string]string{
			ImagePullSyncTimestampAnnotation: time.Now().Format(time.RFC3339),
		}
		secret.Data = map[string][]byte{
			".dockercfg": []byte(fmt.Sprintf(`{"docker.pkg.github.com":{"username":"x","password":"%s","email":"none"}}`, token)),
		}
	}

	if create {
		_, err := secretClient.Create(context.Background(), secret, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		log.Printf("Created %s secret in %s namespace\n", DockerSecretName, namespace.GetName())
	}
	if update {
		_, err := secretClient.Update(context.Background(), secret, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		log.Printf("Updated %s secret in %s namespace\n", DockerSecretName, namespace.GetName())
	}

	serviceAccountClient := g.KubernetesClient.CoreV1().ServiceAccounts(namespace.GetName())

	serviceAccount, err := serviceAccountClient.Get(context.Background(), "default", metav1.GetOptions{})
	if err != nil {
		return err
	}

	referenceExists := false
	ref := v1.LocalObjectReference{
		Name: DockerSecretName,
	}

	for _, secret := range serviceAccount.ImagePullSecrets {
		if secret.Name == DockerSecretName {
			referenceExists = true
			break
		}
	}

	if !referenceExists {
		serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, ref)
	}

	if !referenceExists {
		_, err = serviceAccountClient.Update(context.Background(), serviceAccount, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		log.Printf("Added %s to the %s serviceAccount's list of imagePullSecrets\n", DockerSecretName, serviceAccount.Name)
	}

	return nil
}

// ResyncInstallationNamespaces loads a list of Installations and Repositories
// from GitHub and queues up a call to SyncNamespace for each. Run calls this
// every Config.InstallationResyncInterval
func (g *Geryon) ResyncInstallationNamespaces() {
	log.Println("Resyncing installations...")
	listOptions := &github.ListOptions{PerPage: 100}
	var allInstallations []*github.Installation
	for {
		installations, resp, err := g.GithubAppsClient.Apps.ListInstallations(context.Background(), listOptions)
		if err != nil {
			log.Println(err.Error())
		}
		allInstallations = append(allInstallations, installations...)
		if resp.NextPage == 0 {
			break
		}
		listOptions.Page = resp.NextPage
	}
	for _, installation := range allInstallations {
		err := g.queueReposForInstallation(installation.GetID())
		if err != nil {
			log.Println(err.Error())
		}
	}
}

// ResyncManagedNamespaces loads a list of all namespaces that were created by
// Geryon and queues up a call to SyncNamespace for each every
// Config.NamespaceResyncInterval
func (g *Geryon) ResyncManagedNamespaces() {
	log.Println("Resyncing namespaces...")
	for _, ns := range g.NamespaceInformerCache.List() {
		namespace := ns.(*v1.Namespace)
		if metav1.HasAnnotation(namespace.ObjectMeta, ImagePullSyncAnnotation) &&
			metav1.HasAnnotation(namespace.ObjectMeta, InstallationIDAnnotation) {
			g.queue.Add(fmt.Sprintf("%s:%s", namespace.GetName(), namespace.ObjectMeta.GetAnnotations()[InstallationIDAnnotation]))
		}
	}
}

func (g *Geryon) queueReposForInstallation(installationID int64) error {
	installationClient, err := g.getInstallationClient(installationID)
	if err != nil {
		return err
	}
	listOptions := &github.ListOptions{PerPage: 100}
	var allRepos []*github.Repository
	for {
		repos, resp, err := installationClient.Apps.ListRepos(context.Background(), listOptions)
		if err != nil {
			log.Println(err.Error())
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		listOptions.Page = resp.NextPage
	}
	for _, repo := range allRepos {
		_, exists, err := g.NamespaceInformerCache.GetByKey(repo.GetName())
		if err != nil {
			log.Println(err.Error())
		} else {
			if !exists {
				g.queue.Add(fmt.Sprintf("%s:%d", repo.GetName(), installationID))
			}
		}
	}
	return nil
}
