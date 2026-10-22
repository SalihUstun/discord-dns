package main

import (
	"bufio"
	"os"
	"strings"
)

var defaultDomains = []string{
	"discord.com",
	"discord.gg",
	"discord.co",
	"discord.dev",
	"discord.new",
	"discord.gift",
	"discord.gifts",
	"discord.media",
	"discord.store",
	"discord.tools",
	"discordapp.com",
	"discordapp.net",
	"discordcdn.com",
	"discordstatus.com",
	"discordactivities.com",
	"discordsays.com",
	"discordpartygames.com",
	"dis.gd",
}

type Matcher struct {
	set map[string]struct{}
}

func NewMatcher(domains []string) *Matcher {
	m := &Matcher{set: make(map[string]struct{}, len(domains))}
	for _, d := range domains {
		d = normalize(d)
		if d != "" {
			m.set[d] = struct{}{}
		}
	}
	return m
}

func normalize(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}

func (m *Matcher) Match(name string) bool {
	name = normalize(name)
	for name != "" {
		if _, ok := m.set[name]; ok {
			return true
		}
		i := strings.IndexByte(name, '.')
		if i < 0 {
			return false
		}
		name = name[i+1:]
	}
	return false
}

func loadDomainsFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, sc.Err()
}
