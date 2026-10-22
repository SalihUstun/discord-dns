package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"
)

type Proxy struct {
	matcher   *Matcher
	resolver  *DoHResolver
	splitTLS  bool
	fragDelay time.Duration
	verbose   bool

	targetPort string
}

func (p *Proxy) Serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go p.handle(c)
	}
}

func (p *Proxy) handle(c net.Conn) {
	defer c.Close()

	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	rec, err := readTLSRecord(c)
	if err != nil {
		if p.verbose {
			log.Printf("[proxy] ClientHello okunamadı: %v", err)
		}
		return
	}
	c.SetReadDeadline(time.Time{})

	sni, sniOff, err := parseSNI(rec[5:])
	if err != nil {
		log.Printf("[proxy] SNI bulunamadı: %v", err)
		return
	}

	if !p.matcher.Match(sni) {
		log.Printf("[proxy] listede olmayan domain reddedildi: %s", sni)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	ips, err := p.resolver.LookupA(ctx, sni)
	cancel()
	if err != nil {
		log.Printf("[proxy] %s çözümlenemedi: %v", sni, err)
		return
	}

	port := p.targetPort
	if la, ok := c.LocalAddr().(*net.TCPAddr); ok && port == "" {
		port = fmt.Sprint(la.Port)
	}

	var up net.Conn
	for _, ip := range ips {
		up, err = net.DialTimeout("tcp", net.JoinHostPort(ip.String(), port), 5*time.Second)
		if err == nil {
			break
		}
	}
	if up == nil {
		log.Printf("[proxy] %s sunucusuna bağlanılamadı: %v", sni, err)
		return
	}
	defer up.Close()
	if tc, ok := up.(*net.TCPConn); ok {
		tc.SetNoDelay(true)
	}

	if err := p.writeFragmented(up, rec, sniOff); err != nil {
		log.Printf("[proxy] %s ClientHello gönderilemedi: %v", sni, err)
		return
	}
	if p.verbose {
		log.Printf("[proxy] %s -> %s", sni, up.RemoteAddr())
	}

	pipe(c, up)
}

func (p *Proxy) writeFragmented(w net.Conn, rec []byte, sniOff int) error {
	var chunks [][]byte
	if p.splitTLS {
		payload := rec[5:]
		cut := sniOff
		if cut <= 0 || cut >= len(payload) {
			cut = len(payload) / 2
		}
		r1 := makeRecord(rec[:5], payload[:cut])
		r2 := makeRecord(rec[:5], payload[cut:])
		chunks = [][]byte{r1[:1], r1[1:], r2}
	} else {
		cut := 5 + sniOff
		if sniOff <= 0 || cut >= len(rec) {
			cut = len(rec) / 2
		}
		chunks = [][]byte{rec[:1], rec[1:cut], rec[cut:]}
	}

	for i, ch := range chunks {
		if _, err := w.Write(ch); err != nil {
			return err
		}
		if p.fragDelay > 0 && i < len(chunks)-1 {
			time.Sleep(p.fragDelay)
		}
	}
	return nil
}

func makeRecord(hdr, data []byte) []byte {
	out := make([]byte, 5+len(data))
	out[0] = hdr[0]
	out[1], out[2] = hdr[1], hdr[2]
	binary.BigEndian.PutUint16(out[3:5], uint16(len(data)))
	copy(out[5:], data)
	return out
}

func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	cp := func(dst, src net.Conn) {
		io.Copy(dst, src)
		if tc, ok := dst.(*net.TCPConn); ok {
			tc.CloseWrite()
		}
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done

	deadline := time.Now().Add(30 * time.Second)
	a.SetDeadline(deadline)
	b.SetDeadline(deadline)
	<-done
}

func readTLSRecord(r io.Reader) ([]byte, error) {
	hdr := make([]byte, 5)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return nil, err
	}
	if hdr[0] != 0x16 {
		return nil, fmt.Errorf("TLS handshake değil (tip 0x%02x)", hdr[0])
	}
	n := int(binary.BigEndian.Uint16(hdr[3:5]))
	if n == 0 || n > 16384+2048 {
		return nil, fmt.Errorf("geçersiz kayıt uzunluğu %d", n)
	}
	rec := make([]byte, 5+n)
	copy(rec, hdr)
	if _, err := io.ReadFull(r, rec[5:]); err != nil {
		return nil, err
	}
	return rec, nil
}

func parseSNI(hs []byte) (string, int, error) {
	errShort := errors.New("ClientHello kısa")
	if len(hs) < 4 || hs[0] != 0x01 {
		return "", 0, errors.New("ClientHello değil")
	}
	i := 4 + 2 + 32
	if len(hs) < i+1 {
		return "", 0, errShort
	}
	i += 1 + int(hs[i])
	if len(hs) < i+2 {
		return "", 0, errShort
	}
	i += 2 + int(binary.BigEndian.Uint16(hs[i:]))
	if len(hs) < i+1 {
		return "", 0, errShort
	}
	i += 1 + int(hs[i])
	if len(hs) < i+2 {
		return "", 0, errShort
	}
	extEnd := i + 2 + int(binary.BigEndian.Uint16(hs[i:]))
	i += 2
	if extEnd > len(hs) {
		extEnd = len(hs)
	}
	for i+4 <= extEnd {
		typ := binary.BigEndian.Uint16(hs[i:])
		l := int(binary.BigEndian.Uint16(hs[i+2:]))
		i += 4
		if i+l > extEnd {
			return "", 0, errShort
		}
		if typ == 0 {
			ext := hs[i : i+l]

			if len(ext) < 5 || ext[2] != 0 {
				return "", 0, errors.New("bozuk server_name")
			}
			nl := int(binary.BigEndian.Uint16(ext[3:5]))
			if 5+nl > len(ext) {
				return "", 0, errShort
			}
			name := string(ext[5 : 5+nl])
			return name, i + 5 + nl/2, nil
		}
		i += l
	}
	return "", 0, errors.New("SNI eklentisi yok")
}
