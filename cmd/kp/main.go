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
	"time"

	"gopkg.in/yaml.v3"
)

const (
	lightGreyUnderlined = "\033[37;4m"
	apiURL              = `https://api.kinopio.club`
	usage               = `Work with Kinopio from the command line.

USAGE
  kp <command> <subcommand> [args]

CORE COMMANDS
  i, inbox:   Interact with user inbox
  config:     Open config file using $EDITOR
  space:      Interact with user spaces
  roundrobin: Round robin kinopio cards
  help:       Print this usage message`

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
  ls, list: Print all spaces
  view:     View a space`

	spaceViewUsage = `View a space

USAGE
  kp space view [command] [args]

COMMANDS
  view <id>:          Print all space components
  view <id> box <id>: Print all cards in box`
	roundRobinUsage = `Perform round-robin scheduling on the cards of a box

USAGE
  kp roundrobin [command] [args]

COMMANDS
  set <space_id> <box_id>: Specify the box whose cards you want to schedule
  next:  Move to the next card in the scheduler
  clear: Clear the round-robin state`
)

type Config struct {
	DirPath      string    `yaml:"-"`
	FilePath     string    `yaml:"-"`
	APIKey       string    `yaml:"api_key"`
	InboxSpaceID string    `yaml:"inbox_space_id"`
	Schedule     Scheduler `yaml:"schedule"`
}

type Space struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Cards []Card `json:"cards"`
	Boxes []Box  `json:"boxes"`
}

type Card struct {
	ID      string `json:"id"`
	SpaceID string `json:"SpaceId"`
	Name    string `json:"name"`
	Height  int    `json:"resizeHeight"`
	Width   int    `json:"resizeWidth"`
	X       int    `json: "x"`
	Y       int    `json: "y"`
	Z       int    `json: "z"`
}

type Box struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Height int    `json:"resizeHeight"`
	Width  int    `json:"resizeWidth"`
	X      int    `json: "x"`
	Y      int    `json: "y"`
}

type Topic struct {
	Name     string        `json:"name"`
	Start    time.Time     `json:"start"`
	Duration time.Duration `json:"duration"`
}

type Scheduler struct {
	Topics    []Topic       `json:"topics"`
	Current   int           `json:"current"`
	TimeSlice time.Duration `json:"time_slice"`
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
			//spaceURL := fmt.Sprintf("%s/space/inbox", apiURL)
		case `add`, `a`:
			if len(args) < 4 {
				fmt.Println(inboxUsage)
				return nil
			}
			name := os.Args[3]
			c := Card{
				Name:    name,
				SpaceID: conf.InboxSpaceID,
			}
			if err := AddCardToInbox(c, conf.APIKey); err != nil {
				return fmt.Errorf("failed to add card to inbox: %v", err)
			}
		default:
			return fmt.Errorf("unknown command %q for \"kp inbox\"\n\n%s", os.Args[2], inboxUsage)
		}
	case `space`:
		if len(args) < 3 {
			fmt.Println(spaceUsage)
			return nil
		}
		switch strings.ToLower(os.Args[2]) {
		case `ls`, `list`: // List spaces owned by user
			spaces, err := GetSpaces(conf.APIKey)
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

		case `view`: // View a space
			if len(args) < 4 {
				fmt.Println(spaceViewUsage)
				return nil
			}
			if len(args) > 6 {
				fmt.Fprintf(os.Stderr, "accepts at most 2 args, recieved %d", len(args)-6)
			}

			if len(args) == 4 {
				id := os.Args[3]
				space, err := GetSpace(id, conf.APIKey)
				if err != nil {
					return fmt.Errorf("failed to retrieve user space: %v", err)
				}
				fmt.Printf("%s\n\n", space.Name)

				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				headers := []string{"ID", "CARD NAME"}
				headersWithColor := strings.Join(headers, "\t") + "\n"
				fmt.Fprintf(w, "%v", headersWithColor)
				for _, c := range space.Cards {
					fmt.Fprintf(w, "%s\t%s\n", c.ID, c.Name)
				}
				w.Flush()

				fmt.Println()

				w = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				headers = []string{"ID", "BOX NAME"}
				headersWithColor = strings.Join(headers, "\t") + "\n"
				fmt.Fprintf(w, "%v", headersWithColor)
				for _, b := range space.Boxes {
					fmt.Fprintf(w, "%s\t%s\n", b.ID, b.Name)
				}
				w.Flush()

				return nil
			}

			if len(args) < 6 {
				fmt.Println(spaceViewUsage)
				return nil
			}
			switch strings.ToLower(os.Args[4]) {
			case `box`:
				if len(args) < 6 {
					fmt.Println(spaceUsage)
					return nil
				}
				spaceID := os.Args[3]
				boxID := os.Args[5]
				cards, err := CardsInBox(spaceID, boxID, conf.APIKey)
				if err != nil {
					return fmt.Errorf("failed to retrieve box: %v", err)
				}
				for _, c := range cards {
					fmt.Println(c.Name)
				}
			default:
				return fmt.Errorf("unknown command %q for \"kp space view <ID>\"\n\n%s", os.Args[5], spaceViewUsage)
			}
		default:
			return fmt.Errorf("unknown command %q for \"kp space\"\n\n%s", os.Args[2], spaceViewUsage)
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
	case `roundrobin`, `rr`:
		if len(args) < 3 {
			fmt.Println(roundRobinUsage)
			return nil
		}
		switch strings.ToLower(args[2]) {
		case `set`:
			if len(args) < 5 {
				fmt.Println(roundRobinUsage)
				return nil
			}
			spaceID := os.Args[3]
			boxID := os.Args[4]
			cards, err := CardsInBox(spaceID, boxID, conf.APIKey)
			if err != nil {
				return fmt.Errorf("failed to retrieve box: %v", err)
			}
			if err := setRoundRobin(conf, cards, 0*time.Minute); err != nil {
				return fmt.Errorf("failed to set round-robin box: %v", err)
			}
		case `next`:
			topic, err := conf.Schedule.next(conf)
			if err != nil {
				return fmt.Errorf("failed to retrieve next topic: %v", err)
			}
			fmt.Println(topic.Name)
		case `clear`:
			if err := conf.Schedule.clear(conf); err != nil {
				return fmt.Errorf("failed to clear scheduler: %v", err)
			}
		default:
			return fmt.Errorf("Unknown command: %q.\n%s\n", os.Args[3], usage)
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
		return nil, fmt.Errorf("failed to retrieve user config: %v", err)
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
		return nil, fmt.Errorf("api_key must be set in config file. Use `kp config` to open the config file with your $EDITOR")
	}

	if conf.InboxSpaceID == "" {
		return nil, fmt.Errorf("inbox_space_id must be set in config file. Use `kp config` to open the config file with your $EDITOR")
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

// GetSpaces returns user spaces
func GetSpaces(key string) ([]Space, error) {
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

// GetSpace returns a user space
func GetSpace(id, key string) (Space, error) {
	spaceURL := fmt.Sprintf("%s/space/%s", apiURL, id)

	req, err := http.NewRequest("GET", spaceURL, nil)
	if err != nil {
		return Space{}, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Space{}, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Space{}, fmt.Errorf("failed to read response body: %v", err)
	}

	// Unmarshal the response into a Space
	var space Space
	err = json.Unmarshal(body, &space)
	if err != nil {
		return Space{}, fmt.Errorf("failed to unmarshall JSON: %v", err)
	}

	return space, nil
}

// getBox returns a user box
func getBox(id, key string) (Box, error) {
	boxURL := fmt.Sprintf("%s/box/%s", apiURL, id)

	req, err := http.NewRequest("GET", boxURL, nil)
	if err != nil {
		return Box{}, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", key)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Box{}, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Box{}, fmt.Errorf("failed to read response body: %v", err)
	}

	// Unmarshal the response into a Box
	var box Box
	err = json.Unmarshal(body, &box)
	if err != nil {
		return Box{}, fmt.Errorf("failed to unmarshall JSON: %v", err)
	}

	return box, nil
}

// CardsInBox returns all cards that reside within a given box
func CardsInBox(spaceID, boxID, key string) ([]Card, error) {
	// Get space
	space, err := GetSpace(spaceID, key)
	if err != nil {
		return []Card{}, fmt.Errorf("failed to retrieve space %q: %v", spaceID, err)
	}
	// Get box
	box, err := getBox(boxID, key)
	if err != nil {
		return []Card{}, fmt.Errorf("failed to retrieve box %q: %v", boxID, err)
	}
	// Find cards that are contained within box
	var cards []Card
	for _, c := range space.Cards {
		if isCardInBox(c, box) {
			cards = append(cards, c)
		}
	}
	return cards, nil
}

// isCardInBox returns whether a card is in a box
func isCardInBox(card Card, box Box) bool {
	return card.X >= box.X &&
		card.X+card.Width <= box.X+box.Width &&
		card.Y >= box.Y &&
		card.Y+card.Height <= box.Y+box.Height
}

func color(c, str string) string {
	return c + str + "\033[0m"
}

// setRoundRobin writes the givens cards to the config file
func setRoundRobin(conf *Config, cards []Card, timeSlice time.Duration) error {
	var topics []Topic
	for _, card := range cards {
		t := Topic{
			Name: card.Name,
		}
		topics = append(topics, t)
	}
	if len(topics) > 1 {
		topics[0].Start = time.Now()
	}

	var s Scheduler
	s.Topics = topics
	s.TimeSlice = timeSlice
	conf.Schedule = s
	if err := conf.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save schedule topics: %v", err)
	}
	return nil
}

// next returns the next topic.
func (s *Scheduler) next(conf *Config) (Topic, error) {
	if len(s.Topics) == 0 {
		return Topic{}, fmt.Errorf("no topics found")
	}

	current := s.Topics[s.Current]
	elapsed := time.Since(current.Start)
	if elapsed < s.TimeSlice {
		return current, nil
	}

	s.Current = (s.Current + 1) % len(s.Topics)
	conf.Schedule = *s
	if err := conf.SaveConfig(); err != nil {
		return Topic{}, fmt.Errorf("failed to save schedule topics: %v", err)
	}

	next := s.Topics[s.Current]
	next.Start = time.Now()
	return next, nil
}

// clear clears the schedule
func (s *Scheduler) clear(conf *Config) error {
	conf.Schedule = Scheduler{}
	if err := conf.SaveConfig(); err != nil {
		return fmt.Errorf("failed to save schedule: %v", err)
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

// SaveConfig writes the given config to the file at config file path.
func (c Config) SaveConfig() error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(c.FilePath, data, 0644)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	if err := Run(); err != nil {
		log.Println(err)
	}
}
