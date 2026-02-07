package util

import (
	"fmt"
	"net"
	"net/url"
	"sort"
)

func buildURL(scheme, host string, port int, basePath string) string {
	u := &url.URL{Scheme: scheme, Host: fmt.Sprintf("%s:%d", host, port), Path: basePath}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String()
}

// DiscoverURLs returns a local-loopback URL and LAN URLs for active interfaces.
func DiscoverURLs(bind string, port int, https bool, basePath string) []string {
	scheme := "http"
	if https {
		scheme = "https"
	}
	seen := map[string]struct{}{}
	urls := make([]string, 0, 8)
	appendURL := func(host string) {
		u := buildURL(scheme, host, port, basePath)
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}

	appendURL("127.0.0.1")
	appendURL("localhost")

	if bind != "" && bind != "0.0.0.0" && bind != "::" {
		appendURL(bind)
		sort.Strings(urls)
		return urls
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		sort.Strings(urls)
		return urls
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ip, _, err := net.ParseCIDR(a.String())
			if err != nil {
				continue
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if v4 := ip.To4(); v4 != nil {
				appendURL(v4.String())
			}
		}
	}
	sort.Strings(urls)
	return urls
}
