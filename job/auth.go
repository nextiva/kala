package job

import (
	"context"
	"errors"
	"fmt"

	jwtverifier "github.com/okta/okta-jwt-verifier-golang"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

var (
	username = ""
	password = ""

	Oauth2Config *oauth2.Config           = nil
	Verifier     *jwtverifier.JwtVerifier = nil
)

func SetupAuth(issuer string, audience string, clientId string, clientSecret string, userName string, pwd string,
	tokenUrl string) {
}

func InitAuth() {
	viper.SetDefault("security.oauth2.audience", "api://default")
	issuer := viper.GetString("security.oauth2.issuer")
	clientId := viper.GetString("security.oauth2.clientId")
	clientSecret := viper.GetString("security.oauth2.clientSecret")
	username = viper.GetString("security.oauth2.username")
	password = viper.GetString("security.oauth2.password")
	audience := viper.GetString("security.oauth2.audience")
	tokenUrl := viper.GetString("security.oauth2.tokenUrl")

	if issuer == "" {
		log.Info("No issuer configured. Requests will not be authenticated")
		return
	}
	if clientId == "" {
		log.Info("No client id configured. Requests will not be authenticated")
		return
	}
	if tokenUrl == "" {
		log.Info("No token URL configured. Requests will not be authenticated")
		return
	}
	Oauth2Config = &oauth2.Config{
		ClientID:     clientId,
		ClientSecret: clientSecret,
		Scopes:       []string{"openid", "profile"},
		Endpoint: oauth2.Endpoint{
			TokenURL: tokenUrl,
		},
	}

	toValidate := map[string]string{}
	toValidate["aud"] = audience

	jwtVerifierSetup := jwtverifier.JwtVerifier{
		Issuer:           issuer,
		ClaimsToValidate: toValidate,
	}

	Verifier = jwtVerifierSetup.New()
}

func GetJobToken(ctx context.Context) (string, error) {
	if Oauth2Config != nil && username != "" && password != "" {
		authToken, err := Oauth2Config.PasswordCredentialsToken(ctx, username, password)
		if err != nil {
			msg := fmt.Sprintf("Unable to obtain token for user %s: %v", username, err)
			return "", errors.New(msg)
		}
		if authToken.AccessToken == "" {
			return "", errors.New("Access token was not returned for user" + username)
		}
		return authToken.AccessToken, nil
	}
	return "", nil
}
