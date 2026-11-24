---
title: Görev devri modu
sidebar:
  order: 5
---

OCR deterministik mühendislik işlemlerini (dosya seçimi, kural çözümleme) yürütürken asıl kod incelemesini ana ajan kendi LLM yetenekleriyle yapar. OCR tarafında LLM uç noktası gerekmez.

## Görev devri modu ne zaman kullanılır? {#when-to-use-delegation-mode}

Bu mod Claude Code, Codex, Cursor, Open Code, Qoder gibi, LLM aboneliğinin ana ajanla birlikte sunulduğu abonelik tabanlı yapay zekâ kodlama ajanları için tasarlanmıştır. OCR'ye ayrı model uç noktası yapılandırmak yerine inceleme için ana ajanın mevcut abonelik kotasını kullanırsınız.

Şu durumlarda görev devri modunu kullanın:

1. Kodlama ajanınız abonelikle çalışıyor ve bu kotayı kod incelemesinde kullanmak istiyorsunuz; ek API anahtarı veya model yapılandırması gerekmez.
2. OCR'yi yalnızca dosya filtreleme, kural çözümleme ve dışlama mantığı için kullanmak, tüm LLM akıl yürütmesini ana ajana bırakmak istiyorsunuz.
3. Kendi inceleme adımı için yapılandırılmış girdilere (dosya listesi + kurallar) ihtiyaç duyan özel ajan süreci oluşturuyorsunuz.

## Ön koşullar {#prerequisites}

`ocr` CLI yüklü olmalıdır:

```bash
which ocr || npm install -g @alibaba-group/open-code-review
```

LLM yapılandırması (`ocr config set …` veya ortam değişkenleri) gerekmez; görev devri modunda OCR hiçbir LLM çağrısı yapmaz.

## Beceri / komut yükleme {#install-the-skill-command}

### Claude Code — Komut {#claude-code-command}

```bash
mkdir -p .claude/commands
curl -o .claude/commands/delegate-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md
```

### Herhangi bir ajan — Beceri {#any-agent-skill}

```bash
npx skills add alibaba/open-code-review --skill open-code-review-delegate
```

Bildirimi elle de kopyalayabilirsiniz:

```bash
cp -R /path/to/open-code-review/skills/open-code-review-delegate ~/.claude/skills/
```

## İş akışı {#workflow}

### Adım 1: Önizleme — inceleme kapsamını belirleyin {#step-1-preview-determine-what-to-review}

```bash
ocr delegate preview [--from <ref> [--to <ref>]] [--commit <hash>] [--exclude <patterns>]
```

Çıktılar:

- **mode** — workspace / range / commit
- **referans meta verileri** — from, to, commit, merge\_base
- **İncelenebilir dosya listesi** — yollar, durum, eklenen/silinen satırlar
- **Hariç tutulan dosyalar** — dışlama nedeni ile

Yaygın çağrılar:

| Senaryo | Komut |
|----------|---------|
| Çalışma alanı değişiklikleri | `ocr delegate preview` |
| Dal karşılaştırması | `ocr delegate preview --from main --to feature` |
| Geçerli dalı main ile karşılaştırma | `ocr delegate preview --from main` |
| Tek commit | `ocr delegate preview -c abc123` |

### Adım 2: Dosyaların kurallarını alın {#step-2-get-rules-for-files}

```bash
ocr delegate rule [--from <ref> [--to <ref>]] [--commit <hash>] <path1> <path2> ...
```

1. adımdaki incelenebilir yolları iletin. Çıktı kural içeriğine göre gruplanır; aynı kurala sahip dosyalar tek grupta gösterilerek tekrar önlenir.

### Adım 3: Farkları alın {#step-3-get-diffs}

1. adımdaki mod ve referanslara göre fark komutunu seçin. İnceleme sırasında her Git çağrısında `git --no-pager` kullanın.

Fark komutları, düz yama üretmek için haricî fark programlarını, metin dönüşümünü ve renkleri kapatır.

**Aralık modu** (merge\_base sağlanmış):

```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color <merge_base>..<to> -- "<path>"
```

**Commit modu**:

```bash
git --no-pager show --no-ext-diff --no-textconv --no-color <commit> -- "<path>"
```

**Çalışma alanı modu**:

```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color HEAD -- "<path>"        # tracked files
cat "<path>"                     # new untracked files
```

Bağlamı okumak için:

```bash
git --no-pager log --no-color --oneline -- "<path>"
git --no-pager blame --no-textconv -- "<path>"
git --no-pager show --no-ext-diff --no-textconv --no-color "<ref>:<path>"
```

Büyük farklarda seçili modun komutuna `--output="<absolute-diff-file>"` ekleyin. Depo dışında benzersiz bir mutlak yol seçip üst dizinini oluşturun. Dosya okuma araçlarında aynı yolu kullanın:

```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color --output="<absolute-diff-file>" <merge_base>..<to> -- "<path>"
```

Git 0 koduyla çıktıktan sonra dosyanın tamamını parçalar hâlinde okuyup inceleyin, ardından silin. Git başarısız olursa veya zaman aşımına uğrarsa yeniden deneyin ya da hatayla birlikte `skipped` kaydedin. Beklenmedik biçimde boş olan çıktıyı yeni bir önizlemeyle kontrol edin.

### Adım 4: Her dosyayı inceleyin {#step-4-review-each-file}

İncelenebilir her dosya için:

1. Farkını alın (3. adım).
2. Eşleşen kural grubunu (2. adım) inceleme kontrol listesi olarak kullanın.
3. Gerektiğinde bağlamı araştırarak ayrıntılı inceleme yapın.

Önizlemedeki her `(path, status)` çiftini `reviewed` veya `skipped` olarak kaydedin. Atlanan her girdi için neden belirtin; `total_files`, `reviewed_files`, `skipped_files` ve `coverage_rate` değerlerini raporlayın.

### Adım 5: Raporlayın {#step-5-report}

Bulguları önem derecesine göre sınıflandırın:

- **Critical/High** — hatalar, güvenlik sorunları, veri kaybı riskleri. Her zaman raporlayın.
- **Medium** — performans sorunları, hata işleme eksikleri. Bağlamıyla raporlayın.
- **Low** — biçim ayrıntıları, küçük öneriler. Açıkça değerli değilse sessizce eleyin.

## Alt komut başvurusu {#sub-commands-reference}

| Komut | Amaç |
|---------|---------|
| `ocr delegate preview` | İncelenebilir dosyaları ve mod/referans meta verilerini listeler |
| `ocr delegate rule <path...>` | İnceleme kurallarını içeriklerine göre gruplandırarak çözümler |

## Ortak seçenekler {#shared-flags}

Her iki alt komut da `--from main` ifadesini `--from main --to HEAD` kısaltması olarak kabul eder. Aralık incelemesi `merge-base(from, to)..to` karşılaştırmasını yapar; hazırlanmış, hazırlanmamış ve izlenmeyen değişiklikleri kapsamaz. Çalışma alanı modu için aralık/commit seçeneklerini belirtmeyin. Aynı hedefi kullanmaları için `preview` ve `rule` komutlarına aynı aralık veya commit seçeneklerini verin. Aralık ve commit seçenekleri birlikte kullanılamaz.

| Seçenek | Açıklama |
|------|-------------|
| `--from <ref>` | Aralık modunun kaynak referansı |
| `--to <ref>` | Aralık modunun hedef referansı; `--from` verilmişse varsayılan `HEAD` |
| `-c, --commit <hash>` | Tek commit modu |
| `--repo <path>` | Depo kökü (varsayılan: geçerli çalışma dizini) |
| `--rule <path>` | Özel rule.json yolu |
| `--exclude <patterns>` | Virgülle ayrılmış dışlama desenleri |
| `-b, --background <text>` | İş bağlamı |
| `-B, --background-file <path>` | Markdown dosyasından iş bağlamı (`-b` seçeneğine göre önceliklidir) |

## Diğer entegrasyon modlarıyla karşılaştırma {#comparison-with-other-integration-modes}

| Mod | LLM'yi kim çağırır? | Kullanım alanı |
|------|-------------------|----------|
| [Ajan becerisi](../agent-skill/) | OCR | Ajan `ocr review` çağırır; tüm incelemeyi OCR yürütür |
| [Komut (Claude Code)](../claude-code/) | OCR | Claude Code içinde eğik çizgi komutu; incelemeyi OCR yürütür |
| **Görev devri modu** | Ana ajan | OCR altyapıyı sağlar; incelemeyi ajan yürütür |

## Ayrıca bakın {#see-also}

- [Ajan becerisi](../agent-skill/) — OCR, ajan adına tüm incelemeyi yürütür.
- [Komut (Claude Code)](../claude-code/) — otomatik düzeltmeli eğik çizgi komutu biçimi.
