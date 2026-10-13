# Aegis

**Dil / Language:** [English](README.md) | [**Türkçe**](README_TR.md)

Geçici, şifreli birebir görüşmeler ve dosya paylaşımı için terminal uygulaması. Geçici bir ad seçin, karşı tarafı Aegis içinde arayın ve bağlantı isteğini kabul edin. Hesap, parola veya bağlantı adreslerini iletmek için başka bir uygulama gerekmez.

[Güvenlik politikası](SECURITY.md) · MIT lisansı

## Başlangıç

**Go 1.25.7 veya üzeri** ile:

```sh
git clone https://github.com/rifatardarslan/Aegis.git
cd Aegis
go run ./cmd/aegis
```

Linux/macOS üzerinde `./install.sh`, Windows üzerinde `install.bat` çalıştırabilirsiniz. Kurulum önce kaynaktan derlemeyi dener; başarısız olursa işletim sistemi ve mimariye uygun binary ile `SHA256SUMS` içeren bir GitHub sürümünü indirir. İndirme veya checksum doğrulaması başarısızsa kurulum tamamlanmış sayılmaz. Böyle bir sürüm yayımlanana kadar kaynaktan derleyin. Linux/macOS kurulumu `~/.local/bin` dizinine yapılır ve Bash, Zsh veya Fish başlangıç ayarına PATH satırı otomatik eklenir. Kurulumdan sonra yeni terminal açıp `aegis` çalıştırabilirsiniz. Diğer kabuklarda `~/.profile` kullanılır. Windows kurulumu `%USERPROFILE%\.aegis\bin` dizinine yapılır ve kullanıcı PATH'i güncellenir.

## Geçici görüşme

1. İki kullanıcı da Aegis'i açıp 3–32 karakterlik bir ad seçer. Harf, rakam, `_` ve `-` kullanılabilir.
2. Karşı tarafın adını Aegis içinde arayın. Arama Unicode normalizasyonu ve büyük/küçük harf eşleştirmesi kullanır. Adlar benzersiz veya rezerve değildir; aynı adlı kişiler ayrı geçici kimliklerle listelenir.
3. Ok tuşlarıyla kişiyi seçip Enter ile bağlantı isteği gönderin.
4. Karşı taraf Y ile kabul, N ile ret verir. İstek 60 saniyede sona erer; Esc arama veya giden isteği iptal eder.
5. Şifreli mesajlaşın ve dosya paylaşın. Esc bağlantıyı kapatır; Ctrl+C uygulamadan çıkar. Kaydedilmemiş dosya varsa etkileşimli kapanıştan önce kaydetme veya silme seçeneği sunulur.

Ana ekranda F2 bulunabilirliği açıp kapatır. F3 gelişmiş Peer ID/multiaddr bağlantısını açar. Arama en fazla 30 saniye sürer. Yerel ağda mDNS, internette ada özel Kademlia provider kayıtları ve mevcut libp2p bootstrap/röle düğümleri kullanılır. İnternet bağlantısı ağ koşullarına ve röle erişimine bağlıdır. Bulunabilirliği kapatmak yeni ilanları durdurur ve canlı profili gizler; eski DHT kayıtları ağın kendi sürelerine göre sona erer.

Yeni görüşmede rastgele yeni ağ kimliği ve geçici uygulama anahtarları üretilir. Görünen ad uygulama açıkken RAM'de yeniden kullanılabilir. Kalıcı hesap, kişi listesi, çevrimdışı mesaj ve sohbet veritabanı yoktur.

## Dosyalar ve kısayollar

| Kısayol | İşlem |
| --- | --- |
| Ctrl+F | Dosya seçici |
| Ctrl+O | Dosya yolu girme |
| Ctrl+S | Doğrulanmış gelen dosyayı kaydetme |
| Ctrl+X | Aktarımı veya dosya hazırlığını iptal etme |
| Sohbette F2 | Parmak izleri ve karşılaştırma kelimeleri |
| Esc | Açık pencereyi iptal veya bağlantıyı kapatma |
| Ctrl+C | Kaydedilmemiş dosya seçeneğiyle çıkış |

Mutlak dosya yolunu sohbet girişine yapıştırmak veya sürüklemek de dosya teklifi başlatır.

Sohbette `/help`, `/status`, `/clear`, `/disconnect` ve `/quit` kullanılabilir. Alındı bildirimi, kişinin mesajı okuduğu anlamına gelmez. Geçmiş en fazla 1.000 mesaj tutar; eski mesajları okurken yeni mesajlar ekranı aşağı kaydırmaz.

İki yönden birinde, aynı anda tek dosya aktarımı desteklenir. Sınır **100 MB**, parça boyutu 256 KB'tır. Aktarım kimliği, kabul/ACK zaman aşımı, iptal ve tüm dosya için SHA-256 doğrulaması kullanılır. Gönderen aktarma, alıcıda doğrulama ve kaydetme durumlarını ayrı görür. Aynı görüşmede art arda dosya aktarılabilir.

Gelen dosya açıkça kaydedilene kadar RAM'de bekler. Kaydetme hatasında yeniden deneme mümkündür. Var olan dosyanın üzerine yazmak onay gerektirir. Kayıt seçilen dizinde özel izinli geçici dosyada hazırlanır; var olan hedef doğrudan kesilip değiştirilmez. Unix'te yeni dosya izinleri yalnızca sahibine açıktır; Windows'ta erişim dosya sistemi ve ACL kurallarına bağlıdır. Kaydetme sırasında uygulama veya işletim sistemi çökerse geçici kayıt dosyası kalabilir.

## Güvenlik ve sınırlar

- libp2p taşıma katmanında eş kimliklerini doğrular. X25519 anahtar değişimi ve Ed25519 imzaları; isteği, iki Peer ID'yi ve geçici açık anahtarları oturuma bağlar.
- ChaCha20-Poly1305, mesaj başına HKDF-SHA256 anahtar zincirleri ve sıralı sayaçlar kullanılır. Tek gönderim çalışanı şifreleme ve paket yazımını sıraya koyar. Geçersiz paket bağlantıyı sonlandırır.
- Bu yapı **simetrik anahtar zinciridir**, tam Signal Double Ratchet değildir. Görüşme içinde DH ratchet veya anahtar ele geçirildikten sonra iyileşme mekanizması yoktur.
- Kullanıcı adı gerçek kişi kimliğinin kanıtı değildir. Parmak izleri ve transkriptten türetilen altı kelime isteğe bağlıdır; güvenilir ve bağımsız bir yoldan karşılaştırıldığında anlam taşır. Aynı doğrulanmamış görüşmede karşılaştırma bağımsız kimlik doğrulaması sağlamaz.
- Uygulama sohbet veritabanı, ayar dosyası veya disk debug log'u oluşturmaz. `--debug` yalnızca sabit tanılama kategorilerini stderr'e yazar; çıktının dosyaya yönlendirilmesi kullanıcıya bağlıdır.
- Kontrollü kapanışta uygulamanın sahip olduğu anahtar ve dosya byte tamponları temizlenir; geçmiş ve giriş referansları bırakılır. Go, terminal ve işletim sistemi kopyalar, swap, çökme dökümleri veya kayıtlar tutabilir; tüm izlerin kesin silinmesi garanti edilmez.
- Keşif ve röle altyapısı bağlantı metadatasını görebilir. Anonimlik veya dağıtık kayıtların anında silinmesi vaat edilmez. Yakalanamayan sonlandırmalar ve ele geçirilmiş cihazlar temizleme garantisinin dışındadır.

Yeni görüşme protokolü `/aegis/2.0.0`, profil protokolü `/aegis/profile/1.0.0` kullanır. İki tarafın da güncellenmesi gerekir; eski v1'e sessiz geri dönüş yoktur.

```sh
aegis --relay '/ip4/RELAY_IP/tcp/PORT/p2p/RELAY_PEER_ID'
aegis --downloads ./received
aegis --debug
```

Özel röle tercih edilen aday olarak eklenir; genel bootstrap/röle adayları kullanılabilir kalır.

## Geliştirme

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/aegis
bash -n install.sh
```

`internal/p2p` keşif, istek ve şifreli oturumu; `internal/transfer` aktarım durumu ve kaydetmeyi; `internal/tui` etkileşimi yönetir. Arka plan sonuçları oturum kimliğiyle eşleştirilir. Testler yerel eşleri, kabul/ret/iptali, aynı adlı kişileri, eşzamanlı gönderimi, paket çerçevelemeyi, bozuk ve art arda dosyaları, güvenli kaydı ve arayüz geçişlerini kapsar. Gerçek ağlar ve CGNAT üzerinde erişim ayrıca cihazlarla test edilmelidir.
