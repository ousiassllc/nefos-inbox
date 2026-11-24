package aiinput

import (
	"net/url"
	"strings"
)

// HostKey は Base URL の送信先（scheme://ホスト。小文字・既定ポートと末尾のドットを除く。パスは含めない）。
// 空は既定の送信先 def。scheme を含めるので https→http の変更（平文化）も別の送信先になる。
func HostKey(raw, def string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return def
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "invalid:" + raw
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	return u.Scheme + "://" + host
}
