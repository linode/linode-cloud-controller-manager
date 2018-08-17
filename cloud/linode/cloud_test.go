package linode

/*
import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"github.com/linode/linodego"
	"golang.org/x/oauth2"
)

func TestInstances(t *testing.T) {
	client := getClient()
	linodes, err := client.ListInstances(context.TODO(), linodego.NewListOptions(0, ""))
	fmt.Println(linodes, err)
	//9529875

}

func TestInstance(t *testing.T) {
	client := getClient()

	jsonFilter, err := json.Marshal(map[string]string{"label": "lt14-045-079-101-025"})
	if err != nil {
		t.Fatal(err)
	}

	linodes, err := client.ListInstances(context.TODO(), linodego.NewListOptions(0, string(jsonFilter)))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(linodes, err)
}

func TestIps(t *testing.T) {
	client := getClient()
	ips, err := client.ListIPAddresses(context.TODO(), linodego.NewListOptions(0, ""))
	fmt.Println(ips, err)
}

func TestIp(t *testing.T) {
	client := getClient()
	ip, err := client.GetInstanceIPAddresses(context.TODO(), 9529875)
	fmt.Println(ip, err)
}

func getClient() *linodego.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: "tokentoken",
	})

	oauth2Client := &http.Client{
		Transport: &oauth2.Transport{
			Source: tokenSource,
		},
	}

	linodeClient := linodego.NewClient(oauth2Client)
	linodeClient.SetDebug(true)
	return &linodeClient
}
*/
