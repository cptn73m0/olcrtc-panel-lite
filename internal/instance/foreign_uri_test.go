package instance

import "testing"

func TestIsForeignURI(t *testing.T) {
	foreign := []string{
		"vless://90c20a0e-7b17-4669-9ec1-025b03f5e5e6@1.2.3.4:21000?security=reality#nono-1",
		"vmess://eyJhZGQiOiIxLjIuMy40In0=",
		"trojan://pass@host:443?sni=x#t",
		"ss://YWVzOnBhc3M@host:8388#s",
		"hysteria2://pass@host:443#h",
		"hy2://pass@host:443#h",
		"  vless://spaces@host:443#s  ",
	}
	for _, raw := range foreign {
		if !IsForeignURI(raw) {
			t.Errorf("IsForeignURI(%q) = false, want true", raw)
		}
	}
	native := []string{
		"olcrtc://jitsi?datachannel@meet.example/room#key$name",
		"olcrtc://vkcalls?vp8channel@https://vk.ru/call/join/x#k$n",
		"https://sub.example/sub/slug",
		"vless://",
		"",
	}
	for _, raw := range native {
		if IsForeignURI(raw) {
			t.Errorf("IsForeignURI(%q) = true, want false", raw)
		}
	}
}
