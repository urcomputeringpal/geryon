package webhooks

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"k8s.io/client-go/util/workqueue"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v25/github"
)

// Server contains the logic to process webhooks, kinda like probot
type Server struct {
	Port            int
	WebhookSecret   string
	PrivateKeyFile  string
	AppID           int
	GitHubAppClient *github.Client
	tr              *http.RoundTripper
	Queue           workqueue.RateLimitingInterface
}

// GenericEvent contains just enough inforamation about webhook to handle
// authentication
type GenericEvent struct {
	Installation *github.Installation `json:"installation,omitempty"`
}

// Run starts a http server on the configured port
func (s *Server) Run(stopCh <-chan struct{}, shutdownCh chan struct{}) error {
	s.tr = &http.DefaultTransport

	http.HandleFunc("/webhooks", s.handle)
	http.HandleFunc("/healthz", s.health)
	http.HandleFunc("/", s.redirect)
	h := &http.Server{
		Addr: fmt.Sprintf(":%d", s.Port),
	}

	go func() {
		log.Printf("Webhook receiver ready\n")

		if err := h.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	<-stopCh
	log.Printf("Webhook receiver shutting down...\n")

	shutdownContext, cancelFunc := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelFunc()
	defer close(shutdownCh)

	h.Shutdown(shutdownContext)

	log.Printf("Webhook receiver shut down successfully\n")
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, []byte(s.WebhookSecret))
	if err != nil {
		log.Println(err)
		return
	}
	defer r.Body.Close()

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		log.Println(err)
		return
	}

	ge := &GenericEvent{}
	err = json.Unmarshal(payload, &ge)
	if err != nil {
		log.Println(err)
		return
	}

	var installationTransport *ghinstallation.Transport
	if ge.Installation != nil {
		installationTransport, err = ghinstallation.NewKeyFromFile(*s.tr, s.AppID, int(ge.Installation.GetID()), s.PrivateKeyFile)
		if err != nil {
			log.Println(err)
			return
		}
	}

	webhook := &Webhook{
		Event:     event,
		AppID:     &s.AppID,
		Github:    github.NewClient(&http.Client{Transport: installationTransport}),
		AppGitHub: s.GitHubAppClient,
		Queue:     s.Queue,
	}

	// TODO Return a 500 if process returns an error
	webhook.Process()
	return
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	// TODO better health checks
	fmt.Fprintf(w, "hi")
}

func (s *Server) redirect(w http.ResponseWriter, r *http.Request) {
	// TODO automatically generate this redirect
	http.Redirect(w, r, "http://github.com/urcomputeringpal/geryon", 301)
}
