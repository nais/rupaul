package cmd

import (
	"fmt"
	"github.com/ghodss/yaml"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/command/token"
	"github.com/nais/naiserator/pkg/apis/nais.io/v1alpha1"
	"github.com/spf13/cobra"
	"github.com/logrusorgru/aurora"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"math/rand"
	"time"
)

type logWriter struct {
}

func (writer logWriter) Write(bytes []byte) (int, error) {
	return fmt.Print(string(bytes))
}

func getRuPaulQuote() string {
	return quotes[rand.Intn(len(quotes))]
}

func printRuPaulQuote() {
	quote := fmt.Sprintf("👸 Random RuPaul quote: \"%s\"", aurora.Magenta(getRuPaulQuote()))
	log.Printf(quote)
}

type DockerComposeService struct {
	Build string `json:"build"`
	Image string `json:"image"`
	Volumes []string `json:"volumes"`
	Ports []string `json:"ports"`
	Environment map[string]string `json:"environment"`
}

type DockerComposeSpec struct {
	Version string `json:"version"`
	Services map[string]DockerComposeService `json:"services"`
}

var cmdDrag = &cobra.Command{
	Use:   "drag [path-to-naiserator-yaml]",
	Short: "Drag secrets from Vault, and generate a companion docker-compose file",
	Long: `Drag secrets from Vault, and generate a companion docker-compose file.`,
	Args: cobra.MinimumNArgs(1),
	Run: Drag,
}

func Fatalf(message string, args ...interface{}) {
	res := aurora.Red(fmt.Sprintf(message, args))
	prefix := aurora.Red(aurora.Bold("ERROR:"))
	log.Printf("%s %s", prefix, res)
	os.Exit(1)
}

func Drag(cmd *cobra.Command, args []string) {
	// Do Stuff Here

	filename := args[0]
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		Fatalf(err.Error())
	}

	app := v1alpha1.Application{}

	err = yaml.Unmarshal([]byte(data), &app)

	if err != nil {
		Fatalf("error: %v", err)
	}

	service := DockerComposeService{
		Build: ".",
		Image: app.Name,
		Environment: make(map[string]string),
		Ports: []string{},
		Volumes: []string{},
	}

	if app.Spec.Port > 0 {
		port := strconv.Itoa(app.Spec.Port)
		service.Ports = append(service.Ports, port + ":" + port)
	}

	spec := DockerComposeSpec{
		Version: "3",
		Services: make(map[string]DockerComposeService),
	}

	if app.Spec.Vault.Enabled {
		service.Volumes = append(service.Volumes, "${PWD}/secrets:/secrets")
	}

	for _, entry := range app.Spec.Env {
		service.Environment[entry.Name] = entry.Value
	}

	spec.Services[app.Name] = service

	d, err := yaml.Marshal(spec)

	outputDir := "."

	os.MkdirAll(outputDir, os.ModePerm)
	dockerComposePath := filepath.Join(outputDir, "docker-compose.yml")
	ioutil.WriteFile(dockerComposePath, d, 0644)
	log.Printf("Generated %s", aurora.Green(dockerComposePath))

	vaultBaseUrl := "https://vault.adeo.no"
	vaultBaseUrlParsed, err := url.Parse(vaultBaseUrl)
	if err != nil {
		Fatalf("Could not parse Vault base url %s", vaultBaseUrl)
	}

	log.Printf("Fetching secrets from Vault (%s)", vaultBaseUrl)
	c, _ := vaultapi.NewClient(&vaultapi.Config{
		Address:    vaultBaseUrl,
		Timeout: 5 * time.Second,
	})

	// Make a simple ping request to the server, to check for network problems
	_, err = c.RawRequest(&vaultapi.Request{
		Method: "GET",
		URL: vaultBaseUrlParsed,
	})
	if err != nil {
		Fatalf("Could not connect to Vault - network problem? %s", err)
	}

	// Get the vault token from the environment variable, or from the vault CLI token helper.
	vaultToken := os.Getenv("VAULT_TOKEN")
	if len(vaultToken) == 0 {
		helper := &token.InternalTokenHelper{}
		vaultToken, err = helper.Get()
		if len(vaultToken) == 0 {
			Fatalf("Looks like you're not logged in to vault. Run \"vault login -method=oidc\" to login.")
		}
		if err != nil {
			Fatalf("Could not get Vault token, %s", err.Error())
		}
	}

	c.SetToken(vaultToken)
	tokenMeta, err := c.Auth().Token().LookupSelf()
	if err != nil {
		Fatalf("Could not verify the validity of the Vault token - it may be invalid or expired. %s", err)
	}

	for k, v := range tokenMeta.Data {
		if k == "display_name" {
			switch v.(type) {
			case string:
				log.Printf("Logged in as %s", v)
			default:
			}
		}
	}

	policies, err := tokenMeta.TokenPolicies()
	if err != nil {
		Fatalf(err.Error())
	}

	log.Printf("The Vault token has policies %s", strings.Join(policies, ", "))

	for _, entry := range app.Spec.Vault.Mounts {
		log.Printf("Reading secret from Vault: %s", entry.KvPath)
		secret, err := c.Logical().Read(entry.KvPath)
		if err != nil {
			Fatalf(err.Error())
		}
		data := getSecretData(secret)
		for k, v := range data {
			switch val := v.(type) {
			case string:
				log.Printf("Found secret %s/%s", entry.KvPath, k)
				destDir := filepath.Join(outputDir, filepath.FromSlash(entry.MountPath))
				err = os.MkdirAll(destDir, os.ModePerm)
				if err != nil {
					Fatalf("Could not make directory %s: %s", destDir, err.Error())
				}
				filename := filepath.Join(destDir, k)
				err = ioutil.WriteFile(filename, []byte(val), 0644)
				if err != nil {
					Fatalf("Could not write secret to %s: %s", filename, err.Error())
				}
			default:
				Fatalf("Secret %s has invalid type", k, val)
			}
		}
	}
}

var rootCmd = &cobra.Command{
	Use:   "rupaul",
	Short: "The Queen of Nais!",
	Long: `RuPaul helps you run a nais app locally with docker-compose.`,
}



func Execute() {
	rand.Seed(time.Now().Unix()) // initialize global pseudo random generator
	log.SetFlags(0)
	log.SetOutput(new(logWriter))
	rootCmd.AddCommand(cmdDrag)
	printRuPaulQuote()
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// Heuristic; check if we only get two keys ("data" and "metadata") and guess that it's kv version 2.
func getSecretData(secret *vaultapi.Secret) map[string]interface{} {
	if metadata, ok := secret.Data["metadata"]; ok {
		if data, ok := secret.Data["data"]; ok {
			if len(secret.Data) == 2 {
				if reflect.TypeOf(metadata).Kind() == reflect.Map && reflect.TypeOf(data).Kind() == reflect.Map {
					return data.(map[string]interface{})
				}
			}
		}
	}
	return secret.Data
}

var quotes = []string{
	"Now Sashay Away!",
	"And If I fly or if I fall, at least I can say I gave it all!",
	"When you become the image of your own imagination, it's the most powerful thing you could ever do.",
	"We're born naked, and the rest is drag.",
	"I dance to the beat of a different drummer.",
	"All sins are forgiven once you start making a lot of money.",
	"With hair, heels, and attitude, honey, I am through the roof!",
	"When the going gets tough, the tough reinvent.",
	"We are all doing drag. Every single person on this planet is doing it.",
	"The amount of respect you have for others is in direct proportion to how much respect you have for yourself.",
	"It’s as if our culture is addicted to fear and the flat screen is our drug dealer.",
	"Through my observations, it became clear that most of society’s rules and customs are rooted in fear and superstition!",
	"Life is about using the whole box of crayons.",
	"Reading is fundamental.",
	"Drag queens have always taken on that role of spilling the tea - and the tea is the emperor has no clothes!",
	"In our subconscious, we all know we're playing roles.",
	"Life is not to be taken seriously.",
	"It's very easy to look at the world and think this is all so cruel and so mean. It's important to not become bitter from it.",
	"To understand humans, you must study them as a species of animal.",
	"There are only two types of people in the world. There are the people who understand that this is a matrix, and then there are the people who buy it lock, stock and barrel.",
	"The gift you can give to other people is allowing them to give you something.",
	"Live your life in the now, because you get to a certain age and you realize, “Wow, that was fast.",
	"There’s not enough dancing in the world. And the fact that there are no daytime discos right now is indicative of the trouble we’re in as a society.",
	"You have to find a tribe.",
}
