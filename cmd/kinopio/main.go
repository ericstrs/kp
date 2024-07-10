package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	apiURL = `https://api.kinopio.club`
	usage  = `USAGE

	kinopio [command] [arguments]`
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

func Run() error {
	args := os.Args
	if len(args) == 1 {
		return fmt.Errorf("Not enough arguments.\n%s\n", usage)
	}

	conf, err := LoadConfig()
	if err != nil {
		return fmt.Errorf("Failed to load config: %v", err)
	}

	switch strings.ToLower(os.Args[1]) {
	case `inbox`, `i`:
		// Create inbox card
		name := os.Args[2]
		c := Card{
			Name:    name,
			SpaceID: conf.InboxSpaceID,
		}

		// Add card to inbox
		if err := AddCardToInbox(c, conf.APIKey); err != nil {
			return fmt.Errorf("Failed to add card to inbox: %v.\n", err)
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
