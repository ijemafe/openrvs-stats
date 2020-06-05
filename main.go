package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	beacon "github.com/ijemafe/openrvs-beacon"
	registry "github.com/ijemafe/openrvs-registry"
)

var Servers = make([]ServerInfo, 0)

func main() {
	go pollServers()

	http.HandleFunc("/servers", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(Servers)
		if err != nil {
			log.Println("marshal error:", err)
			w.Write([]byte("error"))
		}
		w.Header().Add("Content-Type", "application/json")
		w.Write(b)
	})

	// TODO: Move static files to static file server.
	http.HandleFunc("/index.html", func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadFile("web/index.html")
		if err != nil {
			log.Println("read error:", err)
			w.Write([]byte("error"))
		}
		w.Header().Add("Content-Type", "text/html")
		w.Write(b)
	})
	http.HandleFunc("/style.css", func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadFile("web/style.css")
		if err != nil {
			log.Println("read error:", err)
			w.Write([]byte("error"))
		}
		w.Header().Add("Content-Type", "text/css")
		w.Write(b)
	})
	http.HandleFunc("/stats.js", func(w http.ResponseWriter, r *http.Request) {
		b, err := ioutil.ReadFile("web/stats.js")
		if err != nil {
			log.Println("read error:", err)
			w.Write([]byte("error"))
		}
		w.Header().Add("Content-Type", "application/javascript")
		w.Write(b)
	})

	log.Println("listening on http/8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

func pollServers() {
	for {
		inputs, err := getHostPorts()
		if err != nil {
			log.Println(err)
			continue
		}

		var wg sync.WaitGroup
		var lock = sync.RWMutex{}
		for _, input := range inputs {
			wg.Add(1)

			go func(input Input) {
				info, err := populateBeaconData(input)
				if err != nil {
					log.Println("beacon error:", err)
					wg.Done()
					return
				}
				if info.CurrentPlayers == 0 {
					wg.Done()
					return
				}
				lock.Lock()
				for _, s := range Servers {
					if info.IP == s.IP && info.Port == s.Port {
						wg.Done()
						return
					}
				}
				Servers = append(Servers, info)
				lock.Unlock()
				wg.Done()
			}(input)

		}
		wg.Wait()
		log.Println("server info updated")
		time.Sleep(30 * time.Second)
	}
}

func getHostPorts() ([]Input, error) {
	var inputs = make([]Input, 0)
	resp, err := http.Get("http://64.225.54.237:8080/servers")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	lines := bytes.Split(bytes.TrimSuffix(b, []byte{'\n'}), []byte{'\n'})
	for i := 1; i < len(lines); i++ {
		fields := bytes.Split(lines[i], []byte{','})
		host := string(fields[1])
		portBytes := fields[2]
		port, err := strconv.Atoi(string(portBytes))
		if err != nil {
			log.Println("atoi error:", err)
			continue
		}
		inputs = append(inputs, Input{IP: host, Port: port})
	}
	return inputs, nil
}

func populateBeaconData(input Input) (ServerInfo, error) {
	report, err := beacon.GetServerReport(input.IP, input.Port+1000, 5*time.Second)
	if err != nil {
		return ServerInfo{}, err
	}
	info := ServerInfo{
		ServerName:     report.ServerName,
		CurrentPlayers: report.NumPlayers,
		MaxPlayers:     report.MaxPlayers,
		IP:             report.IPAddress,
		Port:           report.Port,
		MapName:        report.CurrentMap,
		GameMode:       FriendlyGameModes[report.CurrentMode],
		MOTD:           report.MOTD,
	}
	var players = make([]Player, 0)
	for i := 0; i < len(report.ConnectedPlayerNames); i++ {
		var p Player
		p.Name = report.ConnectedPlayerNames[i]
		p.Kills = report.ConnectedPlayerKills[i]
		p.Time = report.ConnectedPlayerTimes[i]
		players = append(players, p)
	}
	info.Players = players
	var maps = make([]Map, 0)
	var limiter int
	if len(report.MapRotation) >= len(report.ModeRotation) {
		limiter = len(report.ModeRotation)
	} else {
		limiter = len(report.MapRotation)
	}
	for i := 0; i < limiter; i++ {
		var m Map
		m.Name = report.MapRotation[i]
		m.Mode = FriendlyGameModes[report.ModeRotation[i]]
		maps = append(maps, m)
	}
	info.Maps = maps
	if registry.GameTypes[report.CurrentMode] == "adv" {
		var pvp PVPSettings
		pvp.AutoTeamBalance = report.AutoTeamBalance
		if report.CurrentMode == "Bomb" {
			pvp.BombTimer = report.BombTimer
		}
		pvp.FriendlyFire = report.FriendlyFire
		pvp.RoundsPerMatch = report.RoundsPerMatch
		pvp.TimePerRound = report.TimePerRound
		pvp.TimeBetweenRounds = report.TimeBetweenRounds
		info.PVPSettings = pvp
	} else {
		var coop CoopSettings
		coop.AIBackup = report.AIBackup
		coop.FriendlyFire = report.FriendlyFire
		coop.TerroristCount = report.NumTerrorists
		coop.RotateMapOnSuccess = report.RotateMapOnSuccess
		coop.RoundsPerMatch = report.RoundsPerMatch
		coop.TimePerRound = report.TimePerRound
		coop.TimeBetweenRounds = report.TimeBetweenRounds
		info.CoopSettings = coop
	}
	return info, nil
}

type Input struct {
	IP   string
	Port int
}

type ServerInfo struct {
	ServerName     string       `json:"server_name"`
	CurrentPlayers int          `json:"current_players"`
	MaxPlayers     int          `json:"max_players"`
	IP             string       `json:"ip_address"`
	Port           int          `json:"port"`
	MapName        string       `json:"current_map"`
	GameMode       string       `json:"game_mode"`
	MOTD           string       `json:"motd"`
	Players        []Player     `json:"players"`
	Maps           []Map        `json:"maps"`
	PVPSettings    PVPSettings  `json:"pvp_settings"`
	CoopSettings   CoopSettings `json:"coop_settings"`
}

type Player struct {
	Name  string `json:"name"`
	Kills int    `json:"kills"`
	Time  string `json:"time"`
}

type Map struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

type PVPSettings struct {
	AutoTeamBalance   bool `json:"auto_team_balance"`
	BombTimer         int  `json:"bomb_timer"`
	FriendlyFire      bool `json:"friendly_fire"`
	RoundsPerMatch    int  `json:"rounds_per_match"`
	TimePerRound      int  `json:"time_per_round"`
	TimeBetweenRounds int  `json:"time_between_rounds"`
}

type CoopSettings struct {
	AIBackup           bool `json:"ai_backup"`
	FriendlyFire       bool `json:"friendly_fire"`
	TerroristCount     int  `json:"terrorist_count"`
	RotateMapOnSuccess bool `json:"rotate_map_on_success"`
	RoundsPerMatch     int  `json:"rounds_per_match"`
	TimePerRound       int  `json:"time_per_round"`
	TimeBetweenRounds  int  `json:"time_between_rounds"`
}

var FriendlyGameModes = map[string]string{
	"RGM_BombAdvMode":           "Bomb",
	"RGM_DeathmatchMode":        "Survival",
	"RGM_EscortAdvMode":         "Escort the Pilot",
	"RGM_HostageRescueAdvMode":  "Hostage",
	"RGM_HostageRescueCoopMode": "Hostage Rescue",
	"RGM_MissionMode":           "Mission",
	"RGM_TeamDeathmatchMode":    "Team Survival",
	"RGM_TerroristHuntCoopMode": "Terrorist Hunt",
}
