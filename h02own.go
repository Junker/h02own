package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Junker/tcp_server"
	"github.com/alexflint/go-arg"
	"github.com/eclipse/paho.mqtt.golang"
	"github.com/jinzhu/configor"
	"github.com/oriser/regroup"
	"log"
	"strconv"
	"time"
)

//arguments
var args struct {
	ConfigPath string `arg:"-c" default:"/etc/h02own/config.yaml" help:"config file path`
	Verbose    bool   `help:"Verbose"`
	Debug      bool   `help:"Debug"`
}

var config struct {
	Port uint   `required:"true"`
	Host string `required:"true" default:"localhost"`

	MQTT struct {
		Host     string `default:"localhost"`
		Port     uint   `default:"1883"`
		User     string
		Password string
		ClientId string `default:"h02own"`
		Topic    string `default:"owntracks"`
	}

	Devices []deviceConfig
}

type deviceConfig struct {
	H02 struct {
		Id uint64 `required:"true"`
	}
	Owntracks struct {
		Tid    string `required:"true"`
		Name   string `required:"true"`
		Device string `required:"true"`
	}
}

type OwntracksData struct {
	Type       string  `json:"_type"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Acc        uint32  `json:"acc"`
	Vel        float64 `json:"vel"`
	Tst        int64   `json:"tst"`
	Created_at int64   `json:"created_a"`
	Conn       string  `json:"conn"`
	Tid        string  `json:"tid"`
	T          string  `json:"t"`
}

type H02Data struct {
	Manufacturer  string
	Id            uint64
	DateTime      time.Time
	GPSDataValid  bool
	Latitude      float64
	Longitude     float64
	SpeedKnots    float64
	Direction     uint16
	VehicleStatus []byte
}

var mqtt_client mqtt.Client

var re = `^\*(?P<manufacturer>\w{2}),(?P<id>\d+),V\d,(?P<hours>\d{2})(?P<minutes>\d{2})(?P<seconds>\d{2}),(?P<gps_valid>[AV])?,` +
	`(?P<latitude>\d{4}\.\d{4}),(?P<latitude_symbol>[NS]),(?P<longitude>\d{5}\.\d{4}),(?P<longitude_symbol>[EW]),(?P<speed>\d+\.?\d*),` +
	`(?P<direction>\d+\.?\d*),(?P<day>\d{2})(?P<month>\d{2})(?P<year>\d{2}),(?P<status>[0-9A-F]{8}),?.*#`
var H02LocationMessageRegex = regroup.MustCompile(re)

var MqttConnectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	log.Fatalf("MQTT Connect lost: %v", err)
}

var MqttConnectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	log.Printf("MQTT Connected")
}

var TcpConnectHandler = func(c *tcp_server.Client) {
	if args.Verbose || args.Debug {
		log.Printf("TCP client Connected from %s", c.Conn().RemoteAddr().String())
	}
}

var TcpNewMessage = func(c *tcp_server.Client, message string) {
	if args.Debug {
		log.Printf("TCP connection(%s) get new message: %s", c.Conn().RemoteAddr().String(), message)
	}

	var data, err = ParseH02Message(message)

	if err != nil {
		return
	}

	var device_cfg *deviceConfig

	for _, cfg := range config.Devices {
		if cfg.H02.Id == data.Id {
			device_cfg = &cfg
		}
	}

	if device_cfg == nil {
		log.Printf("Config for Device ID: %d not found", data.Id)
		return
	}

	var json_data = ConvertH02DataToOwntracksJson(data, device_cfg)

	SendOwntracksMessage(json_data, device_cfg)
}

var TcpConnectLostHandler = func(c *tcp_server.Client, err error) {
	if args.Verbose || args.Debug {
		log.Printf("TCP client(%s) lost connection", c.Conn().RemoteAddr().String())
	}
}

func ConvertH02DataToOwntracksJson(h02data *H02Data, device_cfg *deviceConfig) []byte {

	var ignition_on = getNthBit(h02data.VehicleStatus[2], 5) == 1

	var t = "I"
	if ignition_on {
		t = "i"
	}

	var accuracy uint32 = 0
	if !h02data.GPSDataValid {
		accuracy = 1000 // ????
	}

	var owndata = OwntracksData{
		Type:       "location",
		Lat:        h02data.Latitude,
		Lon:        h02data.Longitude,
		Acc:        accuracy,
		Tst:        h02data.DateTime.Unix(),
		Tid:        device_cfg.Owntracks.Tid,
		Vel:        h02data.SpeedKnots * 1.852, //  1Kn = 1.852km/h
		Created_at: time.Now().Unix(),
		Conn:       "m",
		T:          t}

	var json_data, _ = json.Marshal(owndata)

	return json_data
}

func SendOwntracksMessage(json_data []byte, device_cfg *deviceConfig) {

	var topic string = config.MQTT.Topic + "/" + device_cfg.Owntracks.Name + "/" + device_cfg.Owntracks.Device

	if args.Debug {
		log.Printf("Publish to MQTT topic '%s' data: %s", topic, string(json_data))
	}

	mqtt_client.Publish(topic, 0, true, string(json_data))
}

func ParseH02Message(message string) (*H02Data, error) {
	var matches, err = H02LocationMessageRegex.Groups(message)
	if err != nil {
		if args.Debug {
			log.Printf("received wrong format message: %s", message)
		}
		return nil, errors.New("wrong message format")
	}

	data := new(H02Data)

	data.Manufacturer = matches["manufacturer"]
	data.Id, _ = strconv.ParseUint(matches["id"], 0, 64)

	data.DateTime, _ = time.Parse(time.RFC3339, fmt.Sprintf("20%s-%s-%sT%s:%s:%sZ", matches["year"], matches["month"], matches["day"], matches["hours"], matches["minutes"], matches["seconds"]))

	data.GPSDataValid = matches["gps_valid"] == "A"

	data.Latitude, _ = strconv.ParseFloat(matches["latitude"], 32)
	data.Latitude /= 100
	if matches["latitude_symbol"] == "S" {
		data.Latitude *= -1
	}

	data.Longitude, _ = strconv.ParseFloat(matches["longitude"], 32)
	data.Longitude /= 100
	if matches["longitude_symbol"] == "W" {
		data.Longitude *= -1
	}

	data.SpeedKnots, _ = strconv.ParseFloat(matches["speed"], 32)

	direction, _ := strconv.ParseFloat(matches["direction"], 16)
	data.Direction = uint16(direction)

	status1, _ := strconv.ParseInt(matches["status"][0:2], 16, 16)
	status2, _ := strconv.ParseInt(matches["status"][2:4], 16, 16)
	status3, _ := strconv.ParseInt(matches["status"][4:6], 16, 16)
	status4, _ := strconv.ParseInt(matches["status"][6:8], 16, 16)

	data.VehicleStatus = append(data.VehicleStatus, byte(status1), byte(status2), byte(status3), byte(status4))

	return data, nil
}

func getNthBit(val byte, n uint32) int {
	n = 32 - n
	if 1<<n&val > 0 {
		return 1
	}
	return 0
}

func main() {

	arg.MustParse(&args)

	err := configor.New(&configor.Config{ErrorOnUnmatchedKeys: true, Verbose: args.Verbose, Debug: args.Debug}).Load(&config, args.ConfigPath)

	if err != nil {
		log.Fatalf("Error loading config from: %s, error: %s", args.ConfigPath, err)
	}

	server := tcp_server.New(fmt.Sprintf("%s:%d", config.Host, config.Port))

	server.OnNewClient(TcpConnectHandler)
	server.OnNewMessage(TcpNewMessage)
	server.OnClientConnectionClosed(TcpConnectLostHandler)
	server.MessageTerminator('#')

	mqtt_opts := mqtt.NewClientOptions()
	mqtt_opts.AddBroker(fmt.Sprintf("tcp://%s:%d", config.MQTT.Host, config.MQTT.Port))
	mqtt_opts.SetClientID(config.MQTT.ClientId)
	// mqtt_opts.ConnectRetry = true
	mqtt_opts.OnConnect = MqttConnectHandler
	mqtt_opts.OnConnectionLost = MqttConnectLostHandler

	if config.MQTT.User != "" {
		mqtt_opts.SetUsername(config.MQTT.User)
	}

	if config.MQTT.Password != "" {
		mqtt_opts.SetPassword(config.MQTT.Password)
	}

	mqtt_client = mqtt.NewClient(mqtt_opts)

	if token := mqtt_client.Connect(); token.Wait() && token.Error() != nil {
		panic(fmt.Sprintf("MQTT Error:%s", token.Error()))
	}

	server.Listen()
}
