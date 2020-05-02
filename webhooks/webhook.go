package webhooks

import (
	"fmt"
	"log"
	"reflect"

	"github.com/google/go-github/v25/github"
	"k8s.io/client-go/util/workqueue"
)

// Webhook contains an event payload, metadata, and clients
type Webhook struct {
	Event     interface{}
	Github    *github.Client
	AppID     *int64
	AppGitHub *github.Client
	Queue     workqueue.RateLimitingInterface
}

// Process handles webhook events kinda like Probot does
func (w *Webhook) Process() bool {
	switch e := w.Event.(type) {
	case *github.InstallationRepositoriesEvent:
		err := w.SyncInstallation(w.Event.(*github.InstallationRepositoriesEvent))
		if err != nil {
			log.Printf("%+v\n", err)
			return false
		}
		return true
	default:
		log.Printf("ignoring %s\n", reflect.TypeOf(e).String())
	}
	return false
}

// SyncInstallation queues up work for installations
func (w *Webhook) SyncInstallation(re *github.InstallationRepositoriesEvent) error {
	if re.GetAction() == "added" {
		for _, repo := range re.RepositoriesAdded {
			w.Queue.Add(fmt.Sprintf("%s:%d", repo.GetName(), re.GetInstallation().GetID()))
		}
	}
	return nil
}
