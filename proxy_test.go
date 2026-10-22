package main

import (
	"bytes"
	"crypto/tls"
	"net"
	"testing"
)

func captureClientHello(t *testing.T, sni string) []byte {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		tls.Client(client, &tls.Config{ServerName: sni}).Handshake()
	}()
	rec, err := readTLSRecord(server)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	server.Close()
	return rec
}

func TestParseSNI(t *testing.T) {
	rec := captureClientHello(t, "gateway.discord.gg")
	name, off, err := parseSNI(rec[5:])
	if err != nil {
		t.Fatal(err)
	}
	if name != "gateway.discord.gg" {
		t.Fatalf("SNI = %q", name)
	}

	hs := rec[5:]
	start := bytes.Index(hs, []byte(name))
	if off <= start || off >= start+len(name) {
		t.Fatalf("bölme noktası %d alan adının (%d..%d) dışında", off, start, start+len(name))
	}
}

func TestWriteFragmentedReassembles(t *testing.T) {
	rec := captureClientHello(t, "discord.com")
	_, off, err := parseSNI(rec[5:])
	if err != nil {
		t.Fatal(err)
	}

	for _, split := range []bool{true, false} {
		a, b := net.Pipe()
		p := &Proxy{splitTLS: split}
		go func() {
			p.writeFragmented(a, rec, off)
			a.Close()
		}()
		var got bytes.Buffer
		buf := make([]byte, 4096)
		for {
			n, err := b.Read(buf)
			got.Write(buf[:n])
			if err != nil {
				break
			}
		}

		if !split {
			if !bytes.Equal(got.Bytes(), rec) {
				t.Fatal("TCP bölme veriyi bozdu")
			}
			continue
		}

		var payload []byte
		data := got.Bytes()
		for len(data) > 0 {
			r, err := readTLSRecord(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(r, []byte("discord.com")) {
				t.Fatal("bir kayıt alan adının tamamını içeriyor")
			}
			payload = append(payload, r[5:]...)
			data = data[len(r):]
		}
		if !bytes.Equal(payload, rec[5:]) {
			t.Fatal("TLS kayıt bölme veriyi bozdu")
		}
	}
}

func TestMatcher(t *testing.T) {
	m := NewMatcher(defaultDomains)
	for name, want := range map[string]bool{
		"discord.com.":          true,
		"Gateway.Discord.GG":    true,
		"cdn.discordapp.com":    true,
		"c-ist01.discord.media": true,
		"notdiscord.com":        false,
		"discord.com.evil.org":  false,
		"google.com":            false,
	} {
		if got := m.Match(name); got != want {
			t.Errorf("Match(%q) = %v, istenen %v", name, got, want)
		}
	}
}
