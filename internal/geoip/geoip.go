package geoip

import (
	"net"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

// GeoResult holds the geo-enriched fields for an IP address.
type GeoResult struct {
	CountryCode     string
	CountryName     string
	Region          string
	City            string
	Latitude        float64
	Longitude       float64
	Timezone        string
	ASNOrg          string
	ConnectionClass string // "residential", "mobile", or "datacenter"
}

// Lookup resolves IPs using MaxMind GeoLite2 databases.
type Lookup struct {
	cityDB *geoip2.Reader
	asnDB  *geoip2.Reader
}

// Open opens the city database and optionally the ASN database.
// asnDBPath may be empty; ASN enrichment is skipped when it is.
func Open(cityDBPath, asnDBPath string) (*Lookup, error) {
	city, err := geoip2.Open(cityDBPath)
	if err != nil {
		return nil, err
	}
	l := &Lookup{cityDB: city}
	if asnDBPath != "" {
		asn, err := geoip2.Open(asnDBPath)
		if err != nil {
			city.Close()
			return nil, err
		}
		l.asnDB = asn
	}
	return l, nil
}

// Close releases the database file handles.
func (l *Lookup) Close() {
	if l == nil {
		return
	}
	l.cityDB.Close()
	if l.asnDB != nil {
		l.asnDB.Close()
	}
}

// Lookup resolves a raw address (host:port or bare host) to geo data.
// Returns nil if the address is unparseable or not in the database.
func (l *Lookup) Lookup(rawAddr string) *GeoResult {
	if l == nil {
		return nil
	}
	host, _, err := net.SplitHostPort(rawAddr)
	if err != nil {
		host = rawAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return nil
	}

	city, err := l.cityDB.City(ip)
	if err != nil {
		return nil
	}

	result := &GeoResult{
		CountryCode: city.Country.IsoCode,
		CountryName: city.Country.Names["en"],
		City:        city.City.Names["en"],
		Timezone:    city.Location.TimeZone,
		Latitude:    city.Location.Latitude,
		Longitude:   city.Location.Longitude,
	}
	if len(city.Subdivisions) > 0 {
		result.Region = city.Subdivisions[0].Names["en"]
	}

	if l.asnDB != nil {
		if asn, err := l.asnDB.ASN(ip); err == nil && asn.AutonomousSystemOrganization != "" {
			result.ASNOrg = asn.AutonomousSystemOrganization
			result.ConnectionClass = classifyASN(result.ASNOrg)
		}
	}

	return result
}

// classifyASN infers connection class from ASN organization name heuristics.
func classifyASN(org string) string {
	lower := strings.ToLower(org)
	for _, kw := range []string{
		"amazon", "google", "microsoft", "cloudflare", "digitalocean",
		"linode", "akamai", "fastly", "hetzner", "ovh", "leaseweb",
		"vultr", "contabo", "hosting", "data center", "datacenter",
		"serverius", "internap", "psychz", "quadranet",
	} {
		if strings.Contains(lower, kw) {
			return "datacenter"
		}
	}
	for _, kw := range []string{
		"t-mobile", "vodafone", "at&t", "verizon wireless", "sprint",
		"telefonica", "wireless", "cellular", "mobile network",
	} {
		if strings.Contains(lower, kw) {
			return "mobile"
		}
	}
	return "residential"
}
