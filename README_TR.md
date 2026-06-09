# 🛡️ Aegis

[![Lisans: MIT](https://img.shields.io/badge/Lisans-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20Linux%20%7C%20macOS-informational)](.)

> **Sıfır-Güven (Zero-Trust) · Sadece Bellek (Memory-Only) · P2P Güvenli Terminal Mesajlaşma Uygulaması**
> 
> Aegis; şık bir cyberpunk/hacker estetiğine sahip, tamamen terminal (TUI) için tasarlanmış, askeri düzeyde şifrelemeye ve sıfır veri depolama prensibine dayanan bir uçtan uca güvenli mesajlaşma ve dosya transferi aracıdır.

---

## 📂 Proje Yapısı

```
aegis/
├── cmd/
│   └── aegis/
│       └── main.go              # Uygulama giriş noktası & sinyal yakalama
├── internal/
│   ├── crypto/
│   │   ├── aead.go              # ChaCha20-Poly1305 Seal/Open şifreleme
│   │   ├── handshake.go         # X25519 DH + Ed25519 karşılıklı kimlik doğrulama
│   │   ├── identity.go          # Geçici Ed25519 anahtar çifti & SAS üretimi
│   │   ├── ratchet.go           # Double Ratchet motoru (HKDF-SHA256 KDF zinciri)
│   │   ├── wordlist.go          # SAS görüntüleme için BIP-39 kelime listesi
│   │   ├── zeroize.go           # unsafe bellek sıfırlama (Bytes/Array32/String)
│   │   └── crypto_test.go       # Kriptografik birim testleri
│   ├── p2p/
│   │   ├── host.go              # Argon2id anahtar türetme & libp2p host başlatma
│   │   ├── node.go              # P2PNode: bağlantı, el sıkışma, yaşam döngüsü
│   │   ├── discovery.go         # Kademlia DHT + mDNS eş keşfi
│   │   ├── nat.go               # NAT geçişi için genel röle adresleri
│   │   ├── stream.go            # Uzunluk-önekli çerçeveleme (ReadFrame/WriteFrame)
│   │   └── node_test.go         # P2P birim testleri
│   ├── transfer/
│   │   ├── frame.go             # AegisFrame JSON protokol tanımı
│   │   ├── sender.go            # Akışlı SHA-256 dosya özetleme
│   │   ├── receiver.go          # Güvenli dosya kaydetme (0600 izinleri)
│   │   └── transfer_test.go     # Transfer birim testleri
│   └── tui/
│       ├── model.go             # Kök Bubbletea modeli & ekran yönlendirme
│       ├── dashboard.go         # Ekran 1: parola girişi & eş bağlantısı
│       ├── verify.go            # Ekran 2: parmak izi & SAS doğrulama
│       ├── chat.go              # Ekran 3: şifreli sohbet & dosya transferi
│       ├── styles.go            # Lipgloss renk paleti & stil sabitleri
│       └── banner.go            # ASCII art banner gösterimi
├── install.bat                  # Tek tıkla Windows kurulumu (WDAC bypass)
├── run.sh                       # Tek adımda Linux/macOS başlatıcı
├── Makefile                     # Build, install, dev, debug, vet hedefleri
├── go.mod / go.sum              # Go modül bağımlılıkları
├── LICENSE                      # MIT Lisansı
├── SECURITY.md                  # Güvenlik açığı raporlama politikası
├── README.md                    # Dokümantasyon (İngilizce)
└── README_TR.md                 # Dokümantasyon (Türkçe)
```

---

## 🔒 Güvenlik Mimarisi ve Kriptografi Katmanı

Aegis, hem taşıma ağını hem de yerel depolama alanını güvenilmeyen alanlar olarak kabul eden tam bir sıfır-güven (zero-trust) tehdit modeliyle tasarlanmıştır.

### 1. Geçici ve Sıfır-Depolama Sözü
* **Kesin RAM Sınırları:** Sistemde hiçbir veritabanı, log veya yerel konfigürasyon dosyası oluşturulmaz. Kriptografik anahtarlar, oturum transkriptleri, mesaj geçmişleri ve gönderilen/alınan dosya arabellekleri kesinlikle yalnızca uçucu RAM bellek üzerinde yaşar.
* **Kademeli Bellek Kazıma (Zeroization):** Aktif kriptografik anahtarlar (Double Ratchet kök ve zincir anahtarları, geçici Ed25519 kimlik anahtarları) ve dosya tamponları; normal çıkışlarda, bağlantı kopmalarında veya işletim sistemi sinyallerinde anında `unsafe.StringData` ve `unsafe.Slice` (Go 1.20+) kullanılarak doğrudan bellek adresleri üzerinden sıfırlanır ve `runtime.KeepAlive` derleyici bariyerleri ile bellekten tamamen silinmesi garanti altına alınır.

### 2. Geçici Kimlik ve Kimlik Doğrulamalı Anahtar Değişimi
* **Parola Tabanlı Host Anahtarı:** Başlangıçta Aegis kullanıcıya bir ana parola sorar ve libp2p host özel anahtarını Argon2id algoritmasıyla (3 iterasyon, 64 MB RAM, 4 iş parçacığı) bu paroladan deterministik olarak türetir. Parola, türetim işleminin hemen ardından sınır katmanında (boundary layer) bellekten sıfırlanır.
* **X25519 DH El Sıkışması:** Oturum için ortak bir kök anahtarı türetmek amacıyla manuel olarak geçici X25519 Diffie-Hellman el sıkışması gerçekleştirir. Paylaşılan sır (shared secret), HKDF genişletmesinin hemen ardından `defer` ile bellekten silinir.
* **Ed25519 Kimlik İmzaları:** Anahtar değişimleri, oturum başına dinamik olarak oluşturulan geçici bir Ed25519 anahtar çifti ile imzalanır. El sıkışma transkripti bu imzalarla doğrulanarak araya girme (MitM) saldırıları engellenir.

### 3. Simetrik Double Ratchet Motoru
* **Kusursuz İleriye Yönelik Gizlilik (PFS):** Mesajlar, Signal Double Ratchet protokolünün özelleştirilmiş bir uygulaması kullanılarak şifrelenir. Gönderilen ve alınan her mesaj, HKDF-SHA256 üzerinden benzersiz, geçici bir mesaj anahtarı türetir ve zinciri ilerletir. KDF ara tamponları ve türetilen mesaj anahtarları, her adımdan sonra `defer` ile sıfırlanır.
* **ChaCha20-Poly1305 AEAD:** Mesaj paketleri ChaCha20-Poly1305 ile simetrik olarak şifrelenir ve doğrulanır. AEAD anahtarı her şifreleme/çözme işleminden sonra sıfırlanır. Şifreleme nonce değerleri monoton mesaj sayaçlarıyla oluşturulur ve İlişkili Veri (AAD), yeniden oynatma (replay) veya kimliğe bürünme saldırılarını engellemek için uzak Peer ID'leri leksikografik olarak bağlar.

### 4. Kanal Dışı (OOB) Kimlik Doğrulama (Anti-MitM)
* **Parmak İzi Karşılaştırma:** Bağlantı kurulduğunda Aegis, doğrulama ekranında SSH tarzı, SHA-256 tabanlı base64 genel kimlik anahtarı parmak izlerini gösterir.
* **Kısa Kimlik Doğrulama Dizisi (SAS):** Genel anahtar özetlerinin (hash) XOR'lanmasıyla elde edilen ve standart BIP-39 kelime listesine eşlenen 3 kelimelik simetrik bir SAS (örn. `DELTA · ECHO · FOXTROT`) gösterir. Kullanıcılar sohbet odasına girmeden önce bu değerleri harici bir güvenli kanal üzerinden doğrular.

---

## 📂 Güvenli Çoklu Dosya Transferi

Aegis, aynı `/aegis/1.0.0` libp2p stream'i üzerinden multiplexed (çoklanmış), yüksek hızlı ve bellek tabanlı dosya transferini destekler.

* **Sürükle-Bırak / Yolu Yapıştır:** Bir dosyayı işletim sistemi dosya gezgininden doğrudan terminal penceresine sürükleyip bırakarak (veya mutlak dosya yolunu kopyalayıp yapıştırarak) ENTER tuşuna basıp güvenli transferi anında başlatabilirsiniz.
* **Stream Hashing & Chunking:** Dosyalar diskten akış halinde okunur, `256 KB` boyutunda standart parçalara ayrılır ve her parça SHA-256 ile özetlenir.
* **RAM Montajı:** Alınan parçalar doğrudan RAM'de birleştirilir. Diske hiçbir geçici dosya yazılmaz.
* **Bütünlük Doğrulaması:** Transfer tamamlandığında alıcı, tam SHA-256 özetini bağımsız olarak hesaplar ve gönderenin bildirdiği özet ile karşılaştırır. Uyumsuzluk durumunda transfer anında reddedilir ve tampon sıfırlanır.
* **Sahibine Özel İzinler:** RAM'de doğrulanmış bütünlüğü olan dosya tamponu, diske yalnızca bir kez ve kesin `0600` (sadece sahibi okuyabilir/yazabilir) izinleriyle kaydedilir.
* **TUI Geri Bildirimi:** Dosya transfer süreci, sistem durumu panelinde anlık aktarım yüzdesini ve parça bütünlük tanılamalarını gösteren dinamik bir ilerleme çubuğuyla (`ScanBar`) görselleştirilir.

---

## 🛠️ P2P Yönlendirme ve NAT Geçişi

Aegis, merkezi bir mesajlaşma sunucusu olmadan tamamen merkeziyetsiz bir ağ topolojisinde çalışır.

* **Taşıma ve Muxing:** Yamux protokolü kullanılarak çoklanmış güvenli taşıma akışları (TCP / QUIC) üzerinden çalışır.
* **Keşif (Discovery):** Global düğümleri Kademlia DHT bootstrap sunucuları üzerinden, yerel ağdaki düğümleri ise multicast DNS (mDNS) kullanarak keşfeder.
* **Dinamik Eş Ön Bellek Yönetimi (WAN/LAN Geçişi):** Düğümler aynı ağdan farklı ağlara taşındığında, eski IP adresleri otomatik olarak yerel ön bellekten temizlenir (`ClearAddrs`) ve DHT üzerinden taze bir yönlendirme sorgusu ile eşin yeni dış/WAN IP adresi bulunarak bağlantı yeniden kurulur.
* **Gelişmiş Röle & Engelleme Koruması (Rate-Limit Protection):** Public IPFS bootstrap düğümlerini kullanarak otomatik devre rölesi (circuit relay) fallback desteği sağlar. Röle istekleri akıllı algoritmalarla (5 dakikalık periyotlarla) sınırlandırılarak rate-limiting engelleri aşılır. İstenirse `--relay` bayrağı ile özel bir röle adresi tanımlanabilir.
* **NAT Delme (Hole-Punching):** AutoNAT ve DCUtR protokolleri ile karmaşık güvenlik duvarlarını ve NAT arkasındaki cihazları uçtan uca birbirine bağlar.

---

## 🚀 Başlangıç & Kurulum Kılavuzu

Aegis; Windows, Linux ve macOS üzerinde tamamen otomatik, tek komutla kurulum desteği sunar. Kurulum betikleri sisteminizde Go yüklü olup olmadığını kontrol eder. Eğer Go yüklü değilse, otomatik olarak sistem paket yöneticisini (winget, apt-get, Homebrew, dnf, pacman) kullanarak Go yüklemeyi dener; derleme mümkün olmazsa en son precompiled (hazır derlenmiş) sürümü GitHub Releases üzerinden otomatik indirip kurar.

### Sistem Gereksinimleri
* Depoyu klonlamak için **Git** gereklidir.
* **Go 1.21+** yüklü olması önerilir (yüklü değilse kurulum sihirbazı otomatik yüklemeyi dener).

```bash
# Depoyu klonlayın
git clone https://github.com/rifatardarslan/Aegis.git
cd aegis
```

---

### 💻 1. Windows Kurulumu (Otomatik)

Aegis'i Windows'a kurmak için proje dizinindeki kurulum betiğini çalıştırmanız yeterlidir:

```powershell
# Kurulumu başlatmak için 'install.bat' dosyasına çift tıklayın veya terminalden çalıştırın:
.\install.bat
```

**Bu Kurulum Sihirbazı Neler Yapar?**
1. **Go Ortam Kontrolü:** Sisteminizde Go'nun yüklü olup olmadığını denetler.
2. **Otomatik Yükleme (winget):** Go eksikse, Windows Paket Yöneticisi (`winget`) aracılığıyla GoLang kurulumunu otomatik başlatır.
3. **Hazır Derlenmiş Sürüm Fallback'i:** Go kurulumu atlanır veya başarısız olursa, GitHub Releases üzerinden en son hazır `aegis.exe` dosyasını indirir.
4. **Ortam Değişkenleri Konfigürasyonu:** Binary dosyasını ve başlatıcıyı `PATH` yolunuza (`GOPATH\bin` veya `~/.aegis/bin`) yerleştirerek komutu global hale getirir.

**Çalıştırma:**
Kurulum tamamlandıktan sonra **yeni** bir Komut İstemi veya PowerShell penceresi açarak şunu yazın:
```powershell
aegis
```

---

### 🐧 2. Linux & macOS Kurulumu (Otomatik)

Linux (Kali, Ubuntu, Debian) ve macOS sistemlerinde kurulumu tek komutla tamamlamak için:

```bash
# Kurulum betiğini çalıştırın:
chmod +x install.sh
./install.sh
```

**Bu Kurulum Sihirbazı Neler Yapar?**
1. **Go Ortam Kontrolü:** Sisteminizde Go'nun yüklü olup olmadığını denetler.
2. **Otomatik Yükleme:** Go eksikse, sistem paket yöneticiniz (`apt-get`, `brew`, `dnf` veya `pacman`) üzerinden otomatik yüklemeye çalışır.
3. **Hazır Derlenmiş Sürüm Fallback'i:** Go yüklemesi başarısız olursa, işletim sisteminize uygun hazır binary dosyasını (`aegis-linux` veya `aegis-macos`) GitHub Releases'ten indirir.
4. **Ortam Değişkenleri Konfigürasyonu:** Binary dosyasını `$HOME/.local/bin` dizinine kopyalar ve gerekirse shell profilinize (`.bashrc` veya `.zshrc`) bu dizini ekler.

**Çalıştırma:**
Kurulum tamamlandıktan sonra terminalinizi yeniden başlatın veya `source ~/.bashrc` (ya da `source ~/.zshrc`) çalıştırıp şunu yazın:
```bash
aegis
```

---

### ⚙️ Çalıştırma Parametreleri (Tüm Platformlar)
Aegis'i çalıştırırken aşağıdaki kullanışlı parametreleri kullanabilirsiniz:

| Parametre | Açıklama |
|-----------|----------|
| `--debug` | Gelişmiş ağ bağlantı loglarını gösterir (kriptografik sırlar **asla** loglanmaz) |
| `--relay "<multiaddr>"` | IPFS bootstrap sunucuları yerine özel bir devre rölesi multiaddr adresi kullanır |

**Örnekler:**
```bash
aegis --debug
aegis --relay "/ip4/1.2.3.4/tcp/4001/p2p/Qm..."
```

---

## 📡 Canlı Test Adımları

1. **Uygulamayı Başlatın:** İki ayrı terminalde `aegis` komutunu çalıştırın.
2. **Kimlik Üretimi:** Her iki kullanıcı da kendi ana parolasını girer. Argon2id ile Libp2p Peer ID deterministik olarak belirlenir.
3. **Bağlantı:** Bir kullanıcı, diğer kullanıcının Peer ID değerini bağlantı alanına girer. Kademlia DHT yönlendirme ve NAT delme sürecini başlatır.
4. **Kimlik Doğrulama Ekranı:** Ekran flaşörlerle uyarır. Karşı tarafın parmak izlerini ve BIP-39 SAS kelimelerini (örn. `DELTA · ECHO · FOXTROT`) harici olarak karşılaştırın. Onaylamak için **Y**'ye, iptal için **N**'ye basın.
5. **Güvenli Sohbet:** Sohbet odasında mesajlaşmaya başlayın. Gönderilen her mesajda double-ratchet sayaçlarının ilerlediğini sağ panelden görebilirsiniz.
6. **Dosya Gönderimi:** Bir dosyayı doğrudan terminalin metin giriş kutusuna sürükleyip bırakın (veya dosya tarayıcısını kullanmak için **Ctrl+F** tuşlarına basın) ve **ENTER**'a basın. Alıcı, gelen teklifi **Y** ile kabul eder. İlerleme çubuğunun dolmasını bekleyin. Transfer tamamlandığında **Ctrl+S** tuşuna basıp kaydetmek istediğiniz dizini veya yolu belirterek dosyayı güvenli bir şekilde kaydedin (dizin belirtilirse Aegis dosya adını otomatik olarak ekler).
7. **Kapatma:** **Ctrl+C** ile veya çıkış tuşuyla uygulamayı kapatın. Bellekteki tüm kriptografik sırlar ve dosya kalıntıları sıfırlanacaktır.

---

## 🛡️ Tehdit Modeli ve Güvenlik Önlemleri

| Tehdit | Güvenlik Önlemi |
|---|---|
| **Pasif Ağ Dinleme (Eavesdropping)** | TLS 1.3 QUIC Yamux taşıma akışları ve ChaCha20-Poly1305 simetrik şifreleme (çift katmanlı şifreleme). |
| **Aktif Araya Girme (MitM)** | Parmak izi eşleştirme, 3 kelimelik Kısa Kimlik Doğrulama Dizisi (SAS) ve el sıkışmada Ed25519 kimlik doğrulaması. |
| **Yeniden Oynatma (Replay) Saldırıları** | Monoton sayaçlara sahip tek kullanımlık AEAD şifreleme nonce değerleri. Eski veya mükerrer paketler reddedilir. |
| **Anahtar Sızıntısı / Ele Geçirilmesi** | Geçici Ed25519 imza anahtarları. Double Ratchet motorunun kullanılan her anahtarı anında silmesi (PFS). |
| **Cold-Boot Bellek Forensics** | Çıkışlarda, kesintilerde veya ağ kopmalarında RAM'deki tüm özel anahtar ve dosya tamponlarının direct unsafe pointer ile anında sıfırlanması. |
| **Parolanın RAM'de Kalması** | Parolanın hiçbir zaman diske yazılmaması ve sınır katmanında (boundary layer) türetim aşamasından hemen sonra bellekten silinmesi. |

---

## 📜 Lisans

Bu proje [MIT Lisansı](LICENSE) ile lisanslanmıştır.
