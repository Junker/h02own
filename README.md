# H02Own

The H02Own is a TCP Server Owntracks gateway for H02 protocol devices.  
H02Own obtains GPS positions from these devices and publishes those over MQTT in OwnTracks JSON format as location objects.

# Supported devices
H02, H-02A, H-02B, TX-2, H-06, H08, GTLT3, NT201, NT202, S31, LK109, LK106, LK208, LK206, LK310, LK206A, LK206B, MI-G6, CC830, CCTR, CCTR-630, AT-18, GRTQ, LK210, PT301, VT900, G91S, LK209C, G-T005, Incutex TK105, RF-V8S, CCRT809, AT-1, LK660, MT-1, CCTR-622G, Amparos S4, LK910, LK700, LK710, RF-16V, Cantrack-G05, Secumore-G05, Sinotrack ST-901, GTRACK4G, XE710, XE800, TK909, XE210, XE103, XE209A, XE209B, XE209C, XE109, XE208, GTR-100, MV720, MV740 

### Build
	go build h02own.go

### Arguments
```
  --configpath CONFIGPATH, -c CONFIGPATH [default: /etc/h02own/config.yaml]
  --verbose              Verbose
  --debug                Debug
```

# Config file
supported YAML, JSON, TOML config files formats

### Config Example (YAML)
```yaml
host: localhost # optional, default: 0.0.0.0 
port: 1122 # required

mqtt:
  host: localhost # required, default: localhost
  port: 1883 # optional, default: 1883
  user: john # optional
  password: 123 # optional
  clientid: myid # optional
  topic: owntracks # optional, default: owntracks. root MQTT topic
  
devices:
  - h02:
      id: 31121432168 # required. Device ID from H02 protocol, usually IMEI of device
    owntracks:
      tid: CA # required
      name: car # required, will be used in MQTT topic e.g. owntracks/car/gps
      device: gps # required, will be used in MQTT topic e.g. owntracks/car/gps
  - h02:
      id: 31121432166
    owntracks:
      tid: BK
      name: motorbike
      device: gps

```
