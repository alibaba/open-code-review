---
title: Oturum görüntüleyicisi
sidebar:
  order: 10
---

`ocr viewer`, geçmiş inceleme oturumlarını tarayıcı arayüzünde gösteren küçük, gömülü HTTP sunucusudur. Haricî bağımlılık gerekmez; OCR'nin her incelemede diske yazdığı JSONL dosyalarını doğrudan okur.

## Başlatma {#launching}

```bash
ocr viewer                       # start and open the browser
ocr viewer --addr :3000          # bind to all interfaces on port 3000
ocr viewer --addr 0.0.0.0:8080   # bind on all interfaces
ocr viewer --open=never          # just print the URL
ocr viewer --open=always         # force it when auto declines (piped output, WSL)
```

Varsayılan adres `localhost:5483` olur. Sunucu ön planda çalışır; `Ctrl+C` durdurur. Her istekte oturumlar `~/.opencodereview/sessions/` altından ihtiyaç anında taranır. Başka terminaldeki inceleme, JSONL dosyası oluşur oluşmaz görünür.

> **DNS yeniden bağlama koruması.** Görüntüleyici `Host` başlığını geri döngü izin listesiyle (`localhost`, `127.0.0.1`, `::1`) kontrol eder. Belirli bağlanma adresi (ör. `--addr 192.168.1.10:5483`) otomatik eklenir; **joker** adresler (`:3000`, `0.0.0.0`, `::`) eklenmez. Bu durumda LAN IP'si veya ana makine adıyla erişim `forbidden host` döndürür. Joker bağlamada erişimi açmak için `OCR_VIEWER_ALLOWED_HOSTS` değişkenine virgülle ayrılmış izinli adları yazın (ör. `OCR_VIEWER_ALLOWED_HOSTS=box.local,192.168.1.10`).

## Tarayıcıyı açma {#opening-the-browser}

`ocr viewer`, sunucu dinlemeye başlayınca varsayılan tarayıcıyı açar. `--open`, genel `--color` ile aynı üç değeri alır:

| Değer | Davranış |
|---|---|
| `auto` (varsayılan) | Yalnızca çalışması beklenen ortamda açar; aşağıya bakın. |
| `always` | Koşulsuz açar. `auto` reddettiği hâlde tarayıcıya erişilebilen durumlar (yönlendirilmiş çıktı, ekranı olmayan WSL) için. |
| `never` | Yalnızca URL'yi yazdırır. |

`auto` modunda şu durumlarda tarayıcı **açılmaz**:

- stdout terminal değildir; çıktı boruya veya dosyaya yönlendirilmiştir.
- `SSH_CONNECTION` ayarlıdır **ve** ekran aktarımı yoktur. `ssh -X` ve `ssh -Y`, `DISPLAY` ayarladığından engellenmez.
- Linux'ta `DISPLAY` ve `WAYLAND_DISPLAY` boştur; ekran sunucusu yoktur.

Bilinçli olarak açılmaması hata gibi görünmesin diye neden hazır satırına eklenir:

```
Viewer ready: http://localhost:5483 (browser not opened: no DISPLAY or WAYLAND_DISPLAY)
```

Unix'te önce `$BROWSER` denenir: iki noktayla ayrılmış komut listesinde her komut URL için `%s` yer tutucusu içerir veya URL'yi son argüman olarak alır. Aksi hâlde platform varsayılanı çalışır: macOS'ta `open`, Linux/BSD'de `xdg-open`, Windows'ta `rundll32`. Tarayıcının açılamaması stderr uyarısıdır, ölümcül hata değildir; sunucu çalışmaya devam eder.

## Dört sayfa {#four-pages}

| URL | İçerik |
|---|---|
| `/` | Diskte oturumu bulunan tüm depolar. |
| `/r/{repo}` | Bir deponun oturumları, en yeniden başlayarak. |
| `/r/{repo}/{sessionID}` | Tek oturumun tüm ayrıntıları. |
| `/r/{repo}/compare` | Aynı deponun iki oturumunun karşılaştırması. |

`{repo}` kodlanmış yoldur: `/` ve `\` yerine `-`, iki nokta yerine `_` kullanılır; disk dizinleriyle aynı kodlama. Genellikle elle yazmak yerine bağlantılara tıklarsınız.

### `/` — Depo listesi {#repository-list}

En az bir oturumu olan her deponun yolu, toplam oturum sayısı, son etkinlik zamanı ve oturumlara götüren `Check` bağlantısı gösterilir. Arama kutusu depo yoluna göre filtreler. Sayfa başına on depo vardır; sağ alttaki denetimle sayfalar arasında geçilir.

### `/r/{repo}` — Depo oturumları {#r-repo-session-list-for-one-repo}

Her oturumda kimlik (UUID), algılanabildiyse dal adı, inceleme modu, model, dosya sayısı, süre, başlangıç zamanı ve bir önceki oturumla karşılaştırmaya götüren `Check` bağlantısı bulunur. Sayfa başına on oturum gösterilir; sağ alttan sayfa değiştirilir.

### `/r/{repo}/{sessionID}` — Oturum ayrıntıları {#r-repo-sessionid-session-detail}

1. **Başlık** — çalışma dizinini, dalı, inceleme kipini, fark aralığını veya commit'i ve modeli gösterir. Oturum bir analiz içeriyorsa bu bilgiler **Review** ve **Analysis** sekmelerinin üzerinde kalır. İncelemenin durumu, dosya sayısı ve süresi ortak başlık yerine **Review** sekmesinin üstünde gösterilir.
2. **Dosya grubu** — incelenen grup başına bir blok. Anlamsal gruplama nedeniyle bir blok ilişkili birkaç dosya içerebilir; başlığı dosya yollarıdır. Her grupta beş görev türü şeridi vardır:

| Görev türü | Ne zaman görünür? |
|---|---|
| `plan_task` | Planlama çalıştıysa: en büyük dosya ≥ `PLAN_MODE_LINE_THRESHOLD` veya 2+ dosya toplamı ≥ `PLAN_MODE_GROUP_LINE_THRESHOLD`. |
| `main_task` | Her grupta; ana inceleme döngüsü, inceleme turu başına bir geçiş. |
| `review_filter_task` | Grup için inceleme sonrası yorum filtreleme çalıştıysa. |
| `memory_compression_task` | active+compress bölgesi %60 / %80 bütçesini aştıysa. |
| `re_location_task` | `code_comment` konumlandırılamadı ve yedek konumlandırma çalıştıysa. |

Her şerit LLM istek-yanıt turu başına bir **görev kartı** içerir. Kartlar görev türüne göre renklidir; baskın aşamalar kolayca görülür.

Uzun listelerde (**File breakdown**, **Files Reviewed**, **Session Tasks**, **Conversations**) sayfa başına yirmi öğe gösterilir. Tek sayfaya sığan bölümlerde sayfalama gizlenir.

### `/r/{repo}/compare` — İki oturumu karşılaştırma {#r-repo-compare-compare-two-sessions}

`ocr session compare` komutunun dört grubunu sayfada gösterir. Oturum listesinin **Action** sütunundaki `Check`, ilgili satırı bir önceki oturumla karşılaştırır; en yeni satır son çalıştırmadan beri değişenleri gösterir. En eski satırda karşılaştırılacak oturum olmadığından `-` görünür.

| Grup | Anlam |
|---|---|
| New | Yalnızca sonraki çalıştırma bildirdi. |
| Persisting | İki çalıştırma da bildirdi. |
| Resolved | Yalnızca önceki çalıştırma bildirdi. Sonraki çalıştırma dosyanın incelemesini tamamladı veya önceki inceleme sonucunu yeniden kullandı. |
| Not reviewed | Yalnızca önceki çalıştırma bildirdi. Sonraki çalıştırmada dosyanın incelemesi başarısız oldu, yarım kaldı veya dosya kapsam dışındaydı. |

CLI boş grupları atlar; sayfa dördünü de gösterir. `Resolved (0)` da bir sonuçtur; kaybolan bölüm bozuk sayfa izlenimi yaratabilir.

Her grupta sayfa başına yirmi bulgu gösterilir; tek sayfada sayfalama gizlenir.

Kaydedilmiş bir tam dosya taramasında `scanPaths` dizisi varsa ve çalıştırma manifesti tanınmıyorsa, her dosyanın kapsamını son kontrol noktası belirler. Başarıyla tamamlanan incelemeler, hiç bulgu üretmese bile kapsama girer. Sonraki oturumda tanınan bir manifest ve tarama kontrol noktası biçimi yoksa, eski karşılaştırma kuralı eşleşmeyen önceki bulguların tümünü Resolved grubuna alır.

Başka bir çifti karşılaştırmak için sorguyu düzenleyin: `/r/{repo}/compare?before=<older session id>&after=<newer session id>`. İki kimlik de URL'deki depoya ait olmalıdır.

İnceleme modları farklıysa `ocr session compare` ile aynı uyarı gösterilir: aynı dosyalar incelenmemiş olabileceğinden gruplar doğrudan karşılaştırılamaz.

## Görev kartının içeriği {#what-s-in-a-task-card}

Genişletmek için karta tıklayın:

- **Başlık satırı** — istek numarası, model, token (`P:` istem / `C:` tamamlama; varsa `CR:` / `CW:` önbellek okuma/yazma), süre ve tur başarısızsa hata rozeti.
- **Response** — akıl yürütme / `thinking` blokları dâhil ham asistan yanıtı.
- **Tool calls** — her araç çağrısının argümanları ve dönen sonucu (daraltılabilir).

Modele gönderilen tam mesaj listesi ve kapsam içindeki araç tanımları kartta **gösterilmez**. Bunlar için JSONL kaydındaki her `llm_request` girdisinin `messages` alanına bakın.

## İnceleme yorumları {#review-comments}

Görev şeritlerinin altında bulgular dosyaya göre gruplanmış **yorum kartlarında** gösterilir: yorum metni, varsa mevcut/önerilen kod, önem ve kategori rozetleri. Filtre çubuğu önem veya kategoriye göre daraltır. Sayfa başına yirmi yorum vardır; tek sayfada sayfalama gizlenir. Önem/kategori değişikliği 1. sayfaya döner. İşaretleme, **Hide marked** veya işaret temizleme, hâlâ mevcutsa geçerli sayfayı korur.

### Düzelttikçe bulguları işaretleme {#marking-findings-as-you-fix-them}

Her kartta **Fixed** / **Ignored** / **Clear** düğmeleri bulunur:

- İşaretler birbirinin yerini alır; **Clear** kaldırır. Durum renkli etikette görünür.
- **Hide marked**, varsayılan olarak açık ve tarayıcı başına hatırlanır; işaretli kartları gizler. Araç çubuğu işaretli/gizli sayısını gösterir. Tümünü görmek için kapatın.
- **Clear all marks**, oturumun tüm işaretlerini sıfırlar.

İşaretler inceleme verisi değil görüntüleyici durumudur; görüntüleyici salt okunur kalır:

- Oturum sayfasına özgü olarak tarayıcının `localStorage` alanında saklanır. JSONL yanına hiçbir şey yazılmaz; yazma API'si yoktur.
- İşaretler tek oturuma **ve tek tarayıcıya** aittir. Başka tarayıcı/makine işaretsiz görür; site depolamasını temizlemek sıfırlar.
- Aynı değişikliği yeniden incelemek, işaretsiz yeni oturum üretir.

## Kullanım alanları {#use-cases}

### “Model bunu neden söyledi?” {#why-did-the-model-say-that}

Terminal çıktısındaki yorumun dosyasını görüntüleyicide bulun, ilgili grubun `main_task` şeridini izleyin. **Tool calls** içinde ilgili `code_comment` bulunan kart yorumu üretmiştir. Response modelin akıl yürütmesini gösterir. Tam istem ve bağlam için JSONL'deki aynı istek numaralı `llm_request` kaydının `messages` alanını açın.

### “Bu dosya neden sessiz kaldı?” {#why-was-this-file-silent}

**Yorumsuz** dosya yalnızca model bilinçli olarak `task_done` çağırdıysa başarılı incelemedir. Şeritte araç çağrıları olup `code_comment` yoksa kasıtlı temiz sonuçtur. Hata kartıyla bitiyorsa sessizlik görünümündeki başarısızlıktır; uyarı olarak belirtin.

### “Sıkıştırma neyi korudu / çıkardı?” {#what-did-compression-keep-drop}

`memory_compression_task` şeridi her sıkıştırma turunu gösterir. Response bölümünde sonuç özeti; girdideki compress bölgesinin XML'i JSONL'deki turun `llm_request.messages` alanındadır. “Model eski bağlamı unuttu” durumunda ilgili ayrıntının sıkıştırmada çıkarılıp çıkarılmadığını kontrol edebilirsiniz.

## Diskte depolama düzeni {#storage-layout-on-disk}

Görüntüleyici şu konumdan okur:

```
~/.opencodereview/sessions/
└── <path-encoded-repo-path>/
    └── <session-id>.jsonl
```

JSONL dosyasının her satırı bir olaydır:

```json
{"type": "llm_request", "filePath": "src/foo.go", "taskType": "main_task", "request_no": 1, "messages": [{"role": "user", "content": "Review this diff…"}], "timestamp": "2026-06-02T10:15:23Z"}
{"type": "llm_response", "filePath": "src/foo.go", "taskType": "main_task", "model": "claude-sonnet-4-6", "content": "Found 2 issues…", "duration_ms": 8421, "usage": {"prompt_tokens": 12450, "completion_tokens": 320}}
{"type": "tool_call", "filePath": "src/foo.go", "tool_name": "file_read", "arguments": "{\"file_path\":\"src/foo.go\",\"start_line\":1,\"end_line\":50}", "result": "File: src/foo.go (Total lines: 220)\nIS_TRUNCATED: false\nLINE_RANGE: 1-50\n1|package foo…", "ok": true, "duration_ms": 14}
```

`filePath`, **grup anahtarını** tutar: tek dosyada tek yol, birden fazla dosyada sıralanıp virgülle birleştirilmiş yollar.

Yalnızca sona ekleme yapılır. Eksik JSONL, oturumun çalışırken sonlandırıldığını gösterir; görüntüleyici elindeki kısmı sunar.

Alan açmak için oturum dosyalarını tümüyle silin; sonraki istekte dizin yeniden oluşturulur.

## Gizlilik {#privacy}

JSONL kayıtları, farktaki kod dâhil LLM'ye gönderilen ve alınan **her şeyi** içerir. Yalnızca makinenizde `~/.opencodereview/` altında bulunur; OCR bunları başka yere yüklemez.

Uzun süre saklamak istemediğiniz kod varsa:

- Oturum dosyalarını düzenli silin veya
- CI'de `--audience agent --format json` çıktısını geçici boruya aktarın ve JSONL kalıcı olmasın diye geçici `HOME` ile çalıştırın.

OpenTelemetry dışa aktarıcısı ayrıdır; istem içeriğini dışa aktarılan izlerden uzak tutmak için [Telemetri](../telemetry/) sayfasına bakın.

## Görüntüleyicinin uygun olmadığı durumlar {#when-the-viewer-is-not-the-right-tool}

- Programatik son işleme (CI, panolar) için `ocr review --format json --audience agent` kullanın; görüntüleyici insanlara yöneliktir.
- Çok sayıda oturumda arama için JSONL üzerinde doğrudan `jq` kullanın. Arayüzde oturumlar arası arama kutusu henüz yoktur.

## Ayrıca bakın {#see-also}

- [Mimari](../architecture/) — beş görev türünün iç işleyişi.
- [Araçlar](../tools/) — `main_task` kartlarındaki araç çağrıları.
