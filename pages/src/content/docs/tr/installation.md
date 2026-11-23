---
title: Yükleme
sidebar:
  order: 4
---

`ocr` CLI'sini yüklemek için desteklenen beş yöntem vardır.

## NPM (önerilen) {#npm-recommended}

#### Yükleme {#install}

```bash
npm install -g @alibaba-group/open-code-review
```

Belirli bir sürüme sabitlemek için:

```bash
npm install -g @alibaba-group/open-code-review@<version>
```

#### Güncelleme {#updating}

NPM ile yüklendiğinde `ocr` varsayılan olarak kendini otomatik günceller (statik ikili dosya bu mekanizmayı kullanmaz). Her `ocr` çalıştırmasında sarmalayıcı arka planda sessizce paket kayıt deposundaki son sürümü kontrol eder. Güncelleme varsa mevcut incelemeyi etkilemeden otomatik yükseltir. Kontroller arasında 18 dakikalık bekleme süresi vardır; `OCR_UPDATE_INTERVAL` ile dakika cinsinden ayarlanabilir.

Otomatik güncellemeyi kapatmak için `OCR_NO_UPDATE` değişkenine boş olmayan bir değer atayın:

```bash
export OCR_NO_UPDATE=1
```

#### Kaldırma {#uninstalling}

```bash
npm uninstall -g @alibaba-group/open-code-review
```

## Homebrew (macOS / Linux) {#homebrew-macos-linux}

```bash
brew install open-code-review
```

Formül, kaynaktan derlenmiş `ocr` ikili dosyasını yükler.

Daha sonra yükseltmek için:

```bash
brew upgrade open-code-review
```

## Yükleme betiği (curl | sh) {#install-script-curl-sh}

GitHub Release ikili dosyasını sağlama toplamını doğrulayarak indiren pratik yükleyici; CI temel imajları ve grafik arayüzü olmayan makineler için uygundur:

```bash
curl -fsSL https://open-codereview.ai/install.sh | sh
```

Üç ortam değişkenini destekler:

| Değişken | Varsayılan | Amaç |
|---|---|---|
| `OCR_INSTALL_DIR` | `/usr/local/bin` | `ocr` ikili dosyasının yerleştirileceği dizin. |
| `OCR_VERSION` | son sürüm | Belirli bir sürüm etiketine sabitleme (ör. `v1.2.3`). |
| `OCR_GITHUB_MIRROR` | *(ayarlanmamış)* | Sürüm ikili dosyasını ve sağlama toplamını bir GitHub ayna alan adı üzerinden indirme (ör. `gh-proxy.com`). |

Betik, `amd64` / `arm64` üzerinde `darwin` ve `linux` sistemlerini destekler.

#### GitHub aynası kullanma {#using-a-github-mirror}

GitHub'a ağ erişiminin yavaş olduğu bölgelerde ikili dosyayı ve sağlama toplamını ayna üzerinden indirmek için `OCR_GITHUB_MIRROR` değişkenini ayna alan adına ayarlayın:

```bash
export OCR_GITHUB_MIRROR='YOUR_MIRROR_DOMAIN'
```

Değer yalnızca alan adı olmalıdır: `https://` şeması veya sonda eğik çizgi bulunmamalıdır (`https://gh-proxy.com/` yerine `gh-proxy.com`). Ayna bir *yol öneki* olarak kullanılır: ikili dosya `https://<mirror>/github.com/alibaba/open-code-review/releases/download/<version>/…` adresinden alınır. Alan adını değiştiren aynalar (örneğin `github.com` yerine `hub.example.org` kullananlar) bu biçime uymaz; yol öneki kullanan bir ayna seçin.

Ayna hem ikili dosyayı hem `sha256sum.txt` sağlama toplamını kapsar. `OCR_VERSION` ayarlı değilse sürüm çözümlemesi yine doğrudan GitHub API'sini kullanır. Bu adımı tamamen atlamak için sürümü sabitleyin:

```bash
export OCR_VERSION='v1.2.3'
```

> **Güvenlik notu:** Ayna üçüncü taraf bir hizmettir. `OCR_GITHUB_MIRROR` ayarlandığında ikili dosya ve `sha256sum.txt` aynı aynadan indirilir. Kötü niyetli bir ayna, değiştirilmiş ikili dosyayla eşleşen sağlama toplamı sunabilir; ayna modunda bütünlük garantisi geçerli değildir. Aynaya güvenmiyorsanız indirilen dosyayı [sürümler sayfasındaki](https://github.com/alibaba/open-code-review/releases) asıl `sha256sum.txt` dosyasıyla doğrulayın.

Windows'ta (PowerShell 5.1+) PowerShell yükleyicisini kullanın:

```powershell
irm https://open-codereview.ai/install.ps1 | iex
```

Aynı `OCR_INSTALL_DIR`, `OCR_VERSION` ve `OCR_GITHUB_MIRROR` değişkenlerini destekler (`$env:OCR_INSTALL_DIR` / `$env:OCR_VERSION` / `$env:OCR_GITHUB_MIRROR` ile ayarlanır). Varsayılan yükleme konumu `%LOCALAPPDATA%\Programs\ocr` dizinidir.

## GitHub Release ikili dosyası {#github-release-binary}

Node.js istemiyorsanız statik ikili dosyayı doğrudan [sürümler sayfasından](https://github.com/alibaba/open-code-review/releases) indirin:

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux x86_64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux ARM64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (AMD64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

Her sürümde ikili dosyaların yanında bütünlüğü doğrulamanız için `sha256sum.txt` yayımlanır:

```bash
curl -LO https://github.com/alibaba/open-code-review/releases/latest/download/sha256sum.txt
shasum -a 256 -c sha256sum.txt --ignore-missing
```

## Kaynaktan derleme {#build-from-source}

Bu yöntem yalnızca OCR üzerinde geliştirme yapıyorsanız veya hazır ikili dosyası bulunmayan bir platform kullanıyorsanız gereklidir.

#### Ön koşullar {#prerequisites}

- [Go ≥ 1.26.10](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

#### Derleme {#build}

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build              # writes dist/opencodereview (dist/opencodereview.exe on Windows)
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### Başka bir platform için derleme {#build-for-another-platform}

```bash
make build-linux-amd64
make build-linux-arm64
make build-darwin-amd64
make build-darwin-arm64
make build-windows-amd64   # Windows (x86_64)
make build-windows-arm64   # Windows (ARM64)
make build-all          # all six at once
make sha256sum          # also produce sha256sum.txt
```

`make dist`, `clean → build-all → sha256sum` adımlarını çalıştırır ve ikili dosyaların yanına `VERSION` dosyası yazar. Sürüm yayımlama süreci de tam olarak bunu çalıştırır.

#### Testleri çalıştırma {#run-tests}

```bash
make test               # LC_ALL=C go test -v -race -count=1 ./...
```

## Yüklemeyi doğrulama {#verifying-the-install}

İkili dosyayı hangi yöntemle edinmiş olursanız olun:

```bash
ocr version             # prints version + git commit + build date
ocr --help              # top-level usage
ocr review --help       # full review-command flag list
```

“command not found” hatası alırsanız yükleme konumunun `$PATH` içinde bulunduğunu kontrol edin:

```bash
which ocr
echo $PATH
```

## Kabuk tamamlamasını etkinleştirme (isteğe bağlı) {#enable-shell-completion-optional}

`ocr`, bash, zsh, fish ve PowerShell için sekmeyle tamamlamayı destekler.

```bash
# bash
source <(ocr completion bash)

# zsh
ocr completion zsh > "${fpath[1]}/_ocr"
```

fish, PowerShell ve kalıcı kurulum yönergeleri için [CLI başvurusu](../cli-reference/#ocr-completion) sayfasına bakın.

## OCR durum verilerini nerede saklar? {#where-ocr-stores-state}

| Yol | İçerik |
|---|---|
| `~/.opencodereview/config.json` | LLM uç noktası, dil ve telemetri ayarları (`ocr config set` ile yönetilir). |
| `~/.opencodereview/rule.json` | İsteğe bağlı genel inceleme kuralları. |
| `~/.opencodereview/sessions/<encoded-repo-path>/<session-id>.jsonl` | Her inceleme oturumunun akış hâlindeki JSONL kaydı; `ocr viewer` kullanır. |
| `~/.opencodereview/{last-update-check,update.lock,update-available}` | NPM sarmalayıcısının arka plan güncelleme kontrolü durumu. Varsayılan olarak yaklaşık 18 dakikada bir yeni sürüm arar ve yükseltme ipucu gösterir. `OCR_NO_UPDATE=1` ile kapatın veya `OCR_UPDATE_INTERVAL` ile aralığı dakika cinsinden ayarlayın. Statik ikili dosya bunları yazmaz. |
| `<repo>/.opencodereview/rule.json` | İsteğe bağlı proje inceleme kuralları; commit'e eklenebilir. |

OCR, NPM üzerinden geçici ikili dosya indirmesi dışında `~/.opencodereview/` dışına yazmaz. Bu dizini silmek temiz bir kaldırma sağlar.

## Ayrıca bakın {#see-also}

- [Hızlı başlangıç](../quickstart/) — LLM yapılandırın ve ilk incelemenizi çalıştırın.
- [Yapılandırma](../configuration/) — OCR'nin desteklediği tüm ortam değişkenleri ve yapılandırma anahtarları.
- [Katkıda bulunma](../contributing/) — kaynaktan derleme, testler ve OCR geliştirmesi.
