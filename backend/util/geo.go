package util

import (
	"net"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

var (
	geoDB     *geoip2.Reader
	geoOnce   sync.Once
	geoDBPath string
	geoCache  sync.Map // ip string -> location string
)

func InitGeo(path string) {
	geoDBPath = path
}

func LookupIP(ip string) string {
	if v, ok := geoCache.Load(ip); ok {
		return v.(string)
	}
	loc := lookupIPRaw(ip)
	geoCache.Store(ip, loc)
	return loc
}

func lookupIPRaw(ip string) string {
	geoOnce.Do(func() {
		if geoDBPath == "" {
			return
		}
		var err error
		geoDB, err = geoip2.Open(geoDBPath)
		if err != nil {
			geoDB = nil
		}
	})
	if geoDB == nil {
		return ""
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ""
	}
	if parsed.IsLoopback() || parsed.IsPrivate() {
		return "内网"
	}
	record, err := geoDB.City(parsed)
	if err != nil {
		return ""
	}
	country := record.Country.Names["zh-CN"]
	if country == "" {
		country = record.Country.Names["en"]
	}
	city := record.City.Names["zh-CN"]
	if city == "" {
		city = record.City.Names["en"]
	}
	if country == "" {
		return ""
	}
	if city != "" && city != country {
		return country + " " + city
	}
	return country
}
