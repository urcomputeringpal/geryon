package geryon

import (
	"net/http"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v25/github"
)

func (g *Geryon) getInstallationClient(installationID int64) (*github.Client, error) {
	installationTransport, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, g.Config.GitHubAppID, int(installationID), g.Config.GitHubAppPrivateKeyFile)
	if err != nil {
		return nil, err
	}

	return github.NewClient(&http.Client{Transport: installationTransport}), nil
}

func (g *Geryon) getInstallationToken(installationID int64) (string, error) {
	installationTransport, err := ghinstallation.NewKeyFromFile(http.DefaultTransport, g.Config.GitHubAppID, int(installationID), g.Config.GitHubAppPrivateKeyFile)
	if err != nil {
		return "", err
	}

	return installationTransport.Token()
}
