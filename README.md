<p align="center"><img src="assets/icon.png" width="128" alt="Discord DNS"></p>

<h1 align="center">Discord DNS</h1>

<p align="center">Sadece Discord trafiğini engelden geçiren, arka planda çalışan küçük bir program.<br>Diğer internet trafiğine dokunmaz, hızını düşürmez. macOS ve Windows'ta çalışır.</p>

## Kurulum

[Releases](https://github.com/SalihUstun/discord-dns/releases/latest) sayfasından sistemine uygun dosyayı indir.

### Windows

1. `Discord-DNS-Setup.exe` dosyasına çift tıkla ve **Evet**'e bas.
2. Sihirbazda **İleri → Kabul Ediyorum → Bitir**.

"Windows bilgisayarınızı korudu" uyarısı çıkarsa: **Ek bilgi → Yine de çalıştır**.
Güncellemek için yeni sürümün kurulum dosyasını çalıştır. Kaldırmak için: **Ayarlar → Uygulamalar → Discord DNS → Kaldır**.

### Mac

1. `discord-dns-mac.zip` dosyasını aç, **Discord DNS** uygulamasına çift tıkla.
2. **Kur**'a bas ve Mac şifreni gir.

"Discord DNS açılmadı" uyarısı çıkarsa: **Bitti**'ye bas, sonra
**Sistem Ayarları → Gizlilik ve Güvenlik → Yine de Aç**.

---

Kurulumdan sonra Discord'u tamamen kapatıp yeniden aç. Program bilgisayar her açıldığında kendiliğinden başlar.

## Kullanım

Mac'te uygulamayı kurduktan sonra tekrar açarsan **Kaldır / Yeniden Kur** seçenekleri çıkar. Kaldırınca DNS ayarların kurulum öncesi haline döner.

Terminalden de kullanılabilir:

```
discord-dns              kur, kuruluysa menüyü göster
discord-dns status       durum ve son log satırları
discord-dns uninstall    kaldır
discord-dns -v           ayrıntılı log ile kur
```

## Nasıl çalışır

- Bilgisayarda yerel bir DNS sunucusu çalıştırır. Discord adreslerini yerel bir proxy'ye yönlendirir, diğer adresleri olduğu gibi `1.1.1.1` / `8.8.8.8`'e sorar.
- Proxy, Discord bağlantısının ilk paketini parçalara bölerek gönderir; böylece engelleme sistemi (DPI) hangi siteye gidildiğini göremez.
- Discord'un gerçek adresleri şifreli DNS (DoH) ile bulunur.
- Ses ve görüntü doğrudan gider, proxy'den geçmez.

## Kaynaktan derleme

[Go](https://go.dev) ve Windows kurulum dosyası için [NSIS](https://nsis.sourceforge.io) (`brew install makensis`) gerekir.

```bash
./build.sh       # dist/ klasörüne Mac ve Windows dosyalarını üretir
go test ./...
```

## Lisans

[MIT](LICENSE). Kullanılan açık kaynak bileşenler: [THIRD_PARTY_LICENSES.txt](THIRD_PARTY_LICENSES.txt).
