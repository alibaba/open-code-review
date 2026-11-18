---
title: Komut (Claude Code eklentisi)
sidebar:
  order: 2
---

Birlikte sunulan komutu yükleyerek OCR'yi [Claude Code](https://docs.anthropic.com/en/docs/claude-code) içinde uçtan uca çalıştırın: farkı inceleyin, bulguları sınıflandırın ve uygulanmaya değer düzeltmeleri otomatik uygulayın.

## Depoda neler var? {#what-ships-in-the-repo}

Depo, [`plugins/open-code-review/claude-code/`](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review/claude-code) altında bir Claude Code eklentisi içerir. Komut istemi [`plugins/open-code-review/claude-code/commands/review.md`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/claude-code/commands/review.md) dosyasındadır ve aşağıdaki iş akışının esas kaynağıdır.

## Yükleme {#install}

### Seçenek 1: Eklenti mağazası (önerilen) {#option-1-plugin-marketplace-recommended}

Bu iki komutu **Claude Code içinde** çalıştırın:

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

Bu işlem `/open-code-review:review` komutunu kaydeder ve `/plugin` üzerinden güncellenebilmesini sağlar.

### Seçenek 2: Komut dosyasını doğrudan kopyalama {#option-2-copy-the-command-file-directly}

Eklenti mağazasını kullanmak istemiyorsanız komut dosyasını doğrudan `.claude/commands/` dizinine yerleştirin. Komut `/open-code-review` adıyla kaydedilir (`:review` son eki olmadan).

**Proje düzeyinde** (ekibin paylaşması için depoyla birlikte commit'e ekleyin):

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

**Kullanıcı düzeyinde** (makinedeki tüm projelerde kullanılabilir):

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

### Komut desteği olan diğer ajanlar {#other-agents-with-command-support}

Komut dosyası tek bir ön bilgi alanı içeren düz Markdown'dır; Claude Code'a özgü değildir. Ajanınız benzer bir **komut** düzenini destekliyorsa (bir dizindeki Markdown istemlerini çağrılabilir komutlar olarak yüklüyorsa), yukarıdaki kopyalama yöntemiyle yükleyin: `open-code-review.md` dosyasını ajanın komut okuduğu dizine yerleştirip kendi çağırma yöntemiyle çalıştırın. İstem gövdesi ajandan bağımsızdır; modele hangi `ocr` seçeneklerini kullanacağını ve çıktıyı nasıl değerlendireceğini anlatır.

> **İpucu:** Ayrı bir LLM yapılandırmak istemiyorsanız [Yetkilendirme modu](../delegate/)nu deneyin. Modeli ana ajan (Claude Code) sağlar; ek LLM kurulumu gerekmez.

## Kullanım {#use}

Claude Code'da komutu adıyla çağırın. Eklenti mağazasından yüklediyseniz `/open-code-review:review`, dosyayı doğrudan kopyaladıysanız `/open-code-review` kullanın:

```
/open-code-review:review
/open-code-review:review review this PR against main
/open-code-review:review focus on race conditions in commit abc123
```

İstem, isteğinizi çözümleyerek uygun `ocr review` seçeneklerini seçer: argüman yoksa çalışma alanı modu (hazırlanmış, hazırlanmamış ve izlenmeyen değişiklikler), commit belirtilmişse `--commit`, dal aralığı belirtilmişse `--from` / `--to`. OCR seçeneklerini doğrudan da geçebilirsiniz (ör. `/open-code-review:review --commit abc123` veya `--from main --to feature`).

## Komut ne yapar? {#what-the-command-does}

Komut istemi üç adımdan oluşur:

1. **İncelemeyi çalıştırma.** İsteğinizden çıkarılan seçeneklerle `ocr review --audience agent` çağırır. Gereksinim bağlamı verdiyseniz isteğe bağlı `--background` ekler. Çıktı 5 dakikalık zaman aşımıyla alınır.
2. **Filtreleme ve değerlendirme.** Her yorumu **High** / **Medium** / **Low** olarak sınıflandırır. Güveni düşük yorumlar (olası yanlış pozitifler, önemsiz ayrıntılar, bağlamı eksik bulgular) sessizce elenir; kalanlar gösterilir.
3. **Düzeltme.** Uygulanmaya değer High/Medium bulgularını otomatik düzeltir. [Ajan becerisi](../agent-skill/)nden farklı olarak bu komut **varsayılan olarak otomatik düzeltir**; “incele ve düzenle” iş akışı için uygundur, yalnızca farkı göstermek için değil.

Kod değiştirilmeden önce sorulmasını veya değerlendirme ölçütlerinin değiştirilmesini istiyorsanız istemin yerel kopyasını düzenleyin. Claude Code komutları her çağrıda yeniden okur; yeniden başlatma gerekmez.

## Ayrıca bakın {#see-also}

- [Ajan becerisi](../agent-skill/) — SDK düzeyindeki eşdeğer; aynı CLI'yi kullanır, varsayılanları farklıdır (düzeltmeden önce sorar).
