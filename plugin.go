package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mattermost/mattermost-server/v6/model"
	"github.com/mattermost/mattermost-server/v6/plugin"
)

var config Configuration

type Configuration struct {

	/* Admin Room */
	Admin string

	/* API URL */
	ApiURL string

	/* Store the last time we sent a refresh request */
	LastRefresh Timer

	/* Refresh rate for api database reload, reserved for future use */
	Refresh int
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
	Name     string   `json:"name"`
	TX       string   `json:"tx"`
	RX       string   `json:"rx"`
	Tone     string   `json:"tone"`
	Channel  string   `json:"channel"`
	Mode     []string `json:"modes"`
	Location struct {
		Lat       float32 `json:"lat"`
		Lon       float32 `json:"lng"`
		Locator   string  `json:"locator"`
		Placename string  `json:"placename"`
		Region    string  `json:"region"`
	} `json:"location"`
	Keeper      string `json:"keeper"`
	Api_version string `json:"api_version"`
	Updated     struct {
		Human   string `json:"human"`
		Machine int64  `json:"machine"`
	} `json:"updated"`
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

type Timer struct {
	last time.Time
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

	switch resp.StatusCode {
	case 200:
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return []byte{}, fmt.Errorf("Error extracting data")
		}
		client.CloseIdleConnections()
		return body, nil
	case 404:
		return []byte{}, fmt.Errorf("Not found")
	default:
		return []byte{}, fmt.Errorf("Unexpected status code received from server")
	}
}

/* APRS password generator
 * Thanks to Magicbug (Peter 2E0SQL)
 * Code ported to Go
 */
func Aprs(callsign string) string {

	if len(callsign) > 10 {
		return ""
	}

	callsign = strings.ToUpper(callsign)

	hash := 0x73e2

	for i := 0; i < len(callsign); i += 2 {
		hash ^= int(callsign[i]) << 8
		if i+1 < len(callsign) {
			hash ^= int(callsign[i+1])
		}
	}
	hash = hash & 0x7fff
	return fmt.Sprintf("Your APRS password is: %d", hash)
}

/* RepeaterAPI handle and process the request type */
func RepeaterAPI(cmd string, ctype int) string {

	/* Check to see if we should ask Rik's DB to update */
	if time.Since(config.LastRefresh.last) >= time.Duration(config.Refresh)*time.Second {
		if _, err := APIReceiver(config.ApiURL + "/update"); err != nil {
			return "There was an issue updating: " + err.Error()
		}
		/* Last API update time */
		config.LastRefresh.last = time.Now()
	}

	switch ctype {
	case 0:
		var repeater Repeater
		body, err := APIReceiver(config.ApiURL + "/repeater/" + cmd)
		if err != nil {
			return "Error: " + err.Error()
		}

		err = json.Unmarshal(body, &repeater)
		if err != nil {
			return "Error extracting project data response:" + err.Error()
		}

		if repeater.Name == "" {
			return "Repeater not found!"
		}

		return fmt.Sprintf("Name: %s\nMode: %s\nTx: %s\nRx: %s\nTone: %s\nLat: %f\nLon: %f\nLocator: %s\nKeeper: %s\nLast DB update: %s\n\nAPI Provided by Rik M7GMT", repeater.Name, repeater.Mode, repeater.TX, repeater.RX, repeater.Tone, repeater.Location.Lat, repeater.Location.Lon, repeater.Location.Locator, repeater.Keeper, repeater.Updated.Human)

	case 1:
		var repeaters Repeaters
		body, err := APIReceiver(config.ApiURL + "/findbylocator/" + cmd)
		if err != nil {
			return "Error: " + err.Error()
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
			response += fmt.Sprintf("[%d]\nName: %s\nMode: %s\nTx: %s\nRx: %s\nTone: %s\nLat: %f\nLon: %f\nLocator: %s\nKeeper: %s\n\n", i+1, repeaters.Repeaters[i].Name, repeaters.Repeaters[i].Mode, repeaters.Repeaters[i].TX, repeaters.Repeaters[i].RX, repeaters.Repeaters[i].Tone, repeaters.Repeaters[i].Location.Lat, repeaters.Repeaters[i].Location.Lon, repeaters.Repeaters[i].Location.Locator, repeaters.Repeaters[i].Keeper)
		}
		return response + "API Provided by Rik M7GMT"
	}
	return ""
}

/* RepeaterLookup work out if the user is looking for a repeater by name or locator */
func RepeaterLookup(msg string, args *model.CommandArgs) (string, error) {
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

	config.Admin = conf["admin"].(string)
	config.ApiURL = conf["api"].(string)
	//config.Refresh = conf["refresh"].(uint)
	/* Currently hard coded refresh time */
	config.Refresh = 86400
	config.LastRefresh.last = time.Now()

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

		/*
		 * Check to see if user is requesting APRS password
		 */
		if strings.ToUpper(strings.Fields(args.Command)[1]) == "APRS" {
			if len(strings.Fields(args.Command)) > 2 {
				responseMsg = Aprs(strings.Fields(args.Command)[2])
			}
		} else {
			responseMsg, err = RepeaterLookup(callsign, args)
			if err != nil {
				responseMsg = err.Error()
			}
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
