package main

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/urcomputeringpal/geryon"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

func main() {
	stopCh := signals.SetupSignalHandler()

	port, ok := os.LookupEnv("PORT")
	if !ok {
		port = "8080"
	}
	portInt, _ := strconv.Atoi(port)

	webhookSecret, ok := os.LookupEnv("WEBHOOK_SECRET")
	if !ok {
		log.Fatal("WEBHOOK_SECRET required")
	}

	appID, ok := os.LookupEnv("APP_ID")
	if !ok {
		log.Fatal("APP_ID required")
	}
	appIDInt, _ := strconv.Atoi(appID)

	privateKeyFile, ok := os.LookupEnv("PRIVATE_KEY_FILE")
	if !ok {
		log.Fatal("PRIVATE_KEY_FILE required")
	}

	geryon, err := geryon.NewGeryon(geryon.Config{
		WebhookPort:                portInt,
		GitHubAppID:                appIDInt,
		GitHubAppPrivateKeyFile:    privateKeyFile,
		GithubAppWebHookSecret:     webhookSecret,
		Kubeconfig:                 os.Getenv("KUBECONFIG"),
		// TODO configurable
		InstallationResyncInterval: 5*time.Minute,
		NamespaceResyncInterval:    29*time.Minute,
		Threadiness:                4,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
	err = geryon.Run(stopCh)
	if err != nil {
		log.Fatal(err.Error())
	}

}
