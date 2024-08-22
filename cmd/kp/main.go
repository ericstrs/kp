package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

const (
	lightGreyUnderlined = "\033[37;4m"
	apiURL              = `https://api.kinopio.club`
	usage               = `Work with Kinopio from the command line.

USAGE
  kp <command> <subcommand> [args]

CORE COMMANDS
  i, inbox: Interact with user inbox
  space:    Interact with user spaces
  help:     Print this usage message`

	inboxUsage = `Work with Kinopio inbox.

USAGE
  kp inbox <command> [args]

COMMANDS
  view:          View inbox
	a, add <name>: Add new card to inbox`

	spaceUsage = `Work with Kinopio spaces.

USAGE
  kp space <command> [args]

COMMANDS
	ls, list:  Print all spaces
	view <id>: View a space`
)

type Config struct {
	DirPath      string `yaml:"-"`
	FilePath     string `yaml:"-"`
	APIKey       string `yaml:"api_key"`
	InboxSpaceID string `yaml:"inbox_space_id"`
}

type Card struct {
	SpaceID string `json:"spaceId"`
	Name    string `json:"name"`
}

type Space struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func Run() error {
	args := os.Args
	if len(args) == 1 {
		fmt.Println(usage)
		return nil
	}

	conf, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	switch strings.ToLower(os.Args[1]) {
	case `inbox`, `i`:
		if len(args) < 3 {
			fmt.Println(inboxUsage)
			return nil
		}
		switch strings.ToLower(os.Args[2]) {
		case `view`: // TODO: View inbox
		case `add`, `a`:
			// Create inbox card
			name := os.Args[2]
			c := Card{
				Name:    name,
				SpaceID: conf.InboxSpaceID,
			}
			// Add card to inbox
			if err := AddCardToInbox(c, conf.APIKey); err != nil {
				return fmt.Errorf("failed to add card to inbox: %v", err)
			}
		default:
			return fmt.Errorf(`unknown command %q for "kp inbox"\n%s`, os.Args[2], inboxUsage)
		}
	case `space`:
		if len(args) < 3 {
			fmt.Println(inboxUsage)
			return nil
		}
		switch strings.ToLower(os.Args[2]) {
		case `ls`, `list`: // List spaces owned by user
			spaces, err := Spaces(conf.APIKey)
			if err != nil {
				return fmt.Errorf("failed to retrieve user spaces: %v", err)
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			headers := []string{"ID", "NAME"}
			headersWithColor := strings.Join(headers, "\t") + "\n"
			fmt.Fprintf(w, "%v", headersWithColor)
			for _, space := range spaces {
				fmt.Fprintf(w, "%s\t%s\n", space.ID, space.Name)
			}
			w.Flush()
		case `view`: // TODO: View a space
		}
	case `dirs`:
		dirs := conf.Dirs()
		for _, d := range dirs {
			fmt.Println(d)
		}
	case `config`:
		editor := os.Getenv("EDITOR")
		if err := conf.OpenConfig(editor); err != nil {
			return fmt.Errorf("Failed to open config file: %v", err)
		}
	case `help`:
		fmt.Println(usage)
	default:
		return fmt.Errorf("Unknown command: %q.\n%s\n", os.Args[1], usage)
	}

	return nil
}

// LoadConfig loads in the config file.
func LoadConfig() (*Config, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("error getting user config directly: %v", err)
	}

	dirPath := filepath.Join(dir, `kinopio`)
	filePath := filepath.Join(dirPath, `kinopio.yaml`)

	// If the config file doesn't exist, then create an empty one.
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := createConfig(dirPath, filePath); err != nil {
			return nil, fmt.Errorf("error creating the config file: %v", err)
		}
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	var conf Config
	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("error unmarshalling config file: %v", err)
	}

	if conf.APIKey == "" {
		return nil, fmt.Errorf("api_key must be set in config file. Use `kinopio config` to open the config file with your $EDITOR")
	}

	if conf.InboxSpaceID == "" {
		return nil, fmt.Errorf("inbox_space_id must be set in config file. Use `kinopio config` to open the config file with your $EDITOR")
	}

	conf.DirPath = dirPath
	conf.FilePath = filePath

	return &conf, nil
}

// createConfig creates a config directory if one doesn't exist
// and creates a config file. This function will overwrite an
// existing config file
func createConfig(dirPath, filePath string) error {
	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		return fmt.Errorf("error creating config directory: %v", err)
	}

	defaultConf := Config{
		APIKey:       "",
		InboxSpaceID: "",
	}

	data, err := yaml.Marshal(&defaultConf)
	if err != nil {
		return fmt.Errorf("error marshalling default config: %v", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("error writing default config file: %v", err)
	}

	return nil
}

// AddCardToInbox creates a new card in user's inbox space.
func AddCardToInbox(c Card, key string) error {
	if c.Name == "" {
		return fmt.Errorf("card content cannot be empty")
	}

	cardData, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("error marshalling card data: %v", err)
	}

	inboxURL := fmt.Sprintf("%s/card/to-inbox", apiURL)

	req, err := http.NewRequest("POST", inboxURL, bytes.NewBuffer(cardData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create card, status code: %d", resp.StatusCode)
	}

	return nil
}

// Spaces returns user spaces
func Spaces(key string) ([]Space, error) {
	spaceURL := fmt.Sprintf("%s/user/spaces", apiURL)

	req, err := http.NewRequest("GET", spaceURL, nil)
	if err != nil {
		return []Space{}, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return []Space{}, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []Space{}, fmt.Errorf("failed to read response body: %v", err)
	}

	// Unmarshal the response into a slice of Space
	var spaces []Space
	err = json.Unmarshal(body, &spaces)
	if err != nil {
		return []Space{}, fmt.Errorf("failed to unmarshall JSON: %v", err)
	}

	return spaces, nil
}

func color(c, str string) string {
	return c + str + "\033[0m"
}

// Dirs returns the directories in which kinopio relies on.
func (c Config) Dirs() []string {
	d := []string{}
	d = append(d, c.DirPath)
	return d
}

// OpenConfig opens the config file using the given editor.
func (c Config) OpenConfig(editor string) error {
	cmd := exec.Command(editor, c.FilePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to open config file with editor: %v", err)
	}

	return nil
}

func main() {
	if err := Run(); err != nil {
		log.Println(err)
	}
}
