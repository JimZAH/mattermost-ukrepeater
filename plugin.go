package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"unicode"

	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin"
)

var config Configuration

type Configuration struct {

	/* API URL */
	ApiURL string

	/* Refresh rate for api database reload, reserved for future use */
	Refresh uint64
}

type Plugin struct {
	plugin.MattermostPlugin
	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *Configuration
}

/* Repeater struct to store single repeater instance */
type Repeater struct {
	Name     string `json:"name"`
	TX       string `json:"tx"`
	RX       string `json:"rx"`
	Tone     string `json:"tone"`
	Location struct {
		Lat     float32 `json:"lat"`
		Lon     float32 `json:"lng"`
		Locator string  `json:"locator"`
	} `json:"location"`
}

/* Repeaters struct to store repeater list reponse */
type Repeaters struct {
	Locator      string     `json:"locator"`
	Range        int        `json:"range"`
	Repeaters    []Repeater `json:"repeaters"`
	Message      string     `json:"message"`
	Status       string     `json:"status"`
	RepeaterList []string   `json:"repeater_list"`
}

/* APIReceiver makes the http call to the API server */
func APIReceiver(URL string) ([]byte, error) {
	client := &http.Client{}
	reqs, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		return []byte{}, fmt.Errorf("Error building request")
	}

	reqs.Header.Add("Content-Type", "application/json")
	resp, err := client.Do(reqs)
	if err != nil {
		return []byte{}, fmt.Errorf("Error contacting API")
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return []byte{}, fmt.Errorf("Error extracting data")
	}

	client.CloseIdleConnections()
	return body, nil
}

/* RepeaterAPI handle and process the request type */
func RepeaterAPI(cmd string, ctype int) string {
	switch ctype {
	case 0:
		var repeater Repeater
		body, err := APIReceiver(config.ApiURL + "/repeater/" + cmd)
		if err != nil {
			return "Error extracting project data response: " + err.Error()
		}

		err = json.Unmarshal(body, &repeater)
		if err != nil {
			return "Error extracting project data response:" + err.Error()
		}

		if repeater.Name == "" {
			return "Repeater not found!"
		}

		return fmt.Sprintf("Name: %s\nTx: %s\nRx: %s\nTone: %s\nLat: %f\nLon: %f\nLocator: %s\n\nAPI Provided by Rik M7GMT", repeater.Name, repeater.TX, repeater.RX, repeater.Tone, repeater.Location.Lat, repeater.Location.Lon, repeater.Location.Locator)

	case 1:
		var repeaters Repeaters
		body, err := APIReceiver(config.ApiURL + "/findbylocator/" + cmd)
		if err != nil {
			return "Error extracting project data response: " + err.Error()
		}

		err = json.Unmarshal(body, &repeaters)
		if err != nil {
			return "Error extracting project data response:" + err.Error()
		}

		if len(repeaters.Repeaters) < 1 {
			return "Repeaters not found!"
		}

		var response string
		for i := 0; i < len(repeaters.Repeaters); i++ {
			response += fmt.Sprintf("[%d]\nName: %s\nTx: %s\nRx: %s\nTone: %s\nLat: %f\nLon: %f\nLocator: %s\n\n", i+1, repeaters.Repeaters[i].Name, repeaters.Repeaters[i].TX, repeaters.Repeaters[i].RX, repeaters.Repeaters[i].Tone, repeaters.Repeaters[i].Location.Lat, repeaters.Repeaters[i].Location.Lon, repeaters.Repeaters[i].Location.Locator)
		}
		return response + "API Provided by Rik M7GMT"

	case 2:
		if _, err := APIReceiver(config.ApiURL + "/update"); err != nil {
			return "There was an issue updating: " + err.Error()
		}
	}

	return ""
}

/* RepeaterLookup work out if the user is looking for a repeater by name or locator */
func RepeaterLookup(msg string) (string, error) {
	cmd := strings.TrimPrefix(msg, "ukrepeater ")

	/* If the requested callsign/locator is too short tell the user */
	if len(cmd) < 2 {
		return "", fmt.Errorf("Command is not long enough!: %s", cmd)
	}
	cmd = strings.ToLower(cmd)
	switch cmd[0:2] {
	case "gb", "mb":
		return RepeaterAPI(cmd, 0), nil
	case "jo", "io":
		return RepeaterAPI(cmd, 1), nil
	case "re":
		return RepeaterAPI(cmd, 2), nil
	default:
		return "", fmt.Errorf("Command is not known: %s, Full: %s", cmd[0:2], cmd)
	}
}

func (p *Plugin) OnActivate() error {

	if err := p.API.RegisterCommand(&model.Command{
		Trigger:          "ukrepeater",
		AutoComplete:     true,
		AutoCompleteDesc: "Allows users to search the UKRepeater.net database",
	}); err != nil {
		return err
	}

	conf := p.API.GetPluginConfig()

	config.ApiURL = conf["api"].(string)
	//config.Refresh = conf["refresh"].(uint64)

	p.configuration = &config

	return nil

}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	var responseMsg string
	trigger := strings.TrimPrefix(strings.Fields(args.Command)[0], "/")
	switch trigger {
	case "ukrepeater":
		/* Check the query is of the right length */
		if len(args.Command) < 14 {
			return &model.CommandResponse{
				ResponseType: model.CommandResponseTypeEphemeral,
				Text:         fmt.Sprintf("No John, you need to enter something for me to work with!"),
			}, nil
		}
		var err error
		callsign := strings.Fields(args.Command)[1]
		for _, r := range callsign {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) || len(callsign) > 6 {
				return &model.CommandResponse{
					ResponseType: model.CommandResponseTypeEphemeral,
					Text:         fmt.Sprintf("No Martin!"),
				}, nil
			}
		}
		responseMsg, err = RepeaterLookup(callsign)
		if err != nil {
			responseMsg = err.Error()
		}
	default:
		responseMsg = "Unknown command"
	}
	return &model.CommandResponse{
		ResponseType: model.CommandResponseTypeEphemeral,
		Text:         fmt.Sprintf(responseMsg),
	}, nil
}

func main() {
	plugin.ClientMain(&Plugin{})
}
