---
title: Ajan becerisi
sidebar:
  order: 1
---

OCR'yi çağrılabilir bir beceri olarak kaydedin. Böylece ajan çatısı, doğru seçenekler, ön koşul kontrolleri ve bulgu değerlendirme ölçütleriyle OCR'yi çağırabilir; bunları çağıran tarafta yeniden tanımlamanız gerekmez.

## Depoda neler var? {#what-ships-in-the-repo}

Depo, [`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md) konumunda bir SKILL bildirimi içerir. OCR'yi ön koşul kontrolleri, çağırma iş akışı ve yorum değerlendirme ölçütleri (High/Medium/Low) olan çağrılabilir bir beceri olarak tanımlar.

## Yükleme {#install}

### Seçenek 1: `npx skills add` (önerilen) {#option-1-npx-skills-add-recommended}

Beceriyi kullanmak istediğiniz projenin içinde çalıştırın:

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

Bildirim [beceri kaynağından](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md) alınarak projeye eklenir. Beceri düzenini destekleyen kodlama ajanları bir sonraki çağrıda bunu algılar. Son sürüme güncellemek için komutu yeniden çalıştırın.

> **Ön koşul:** İkili dosya `PATH` üzerinde yoksa beceri ilk çalıştırmada `ocr` CLI'sini kendisi yükler (`npm install -g @alibaba-group/open-code-review`). Aşağıdaki [Beceri ne yapar?](#what-the-skill-does) bölümüne bakın. LLM'yi ise **önceden yapılandırmanız gerekir**; beceri bunu sizin yerinize yapamaz ve durup sizden ister. [Yapılandırma](../../configuration/) sayfasına bakın.

### Seçenek 2: Elle kopyalama (sistem genelinde) {#option-2-manual-copy-system-wide}

Proje başına yüklemek yerine genel kullanım için klasörü beceri dizininize kopyalayın:

```bash
mkdir -p ~/.claude/skills
cp -R /path/to/open-code-review/skills/open-code-review ~/.claude/skills/
```

Böylece beceri makinedeki tüm projelerde kullanılabilir.

## Beceri ne yapar? {#what-the-skill-does}

SKILL.md bir istemdir: çağıran ajan dosyayı yüklediğinde adımları kendisi yürütür. Tek bir `/open-code-review` (veya eşdeğeri) isteği şu şekilde ilerler:

1. **Ön koşul kontrolü.** CLI'nin `PATH` üzerinde olduğunu doğrulamak için `which ocr`, LLM'ye erişilebildiğini doğrulamak için `ocr llm test` çalıştırılır.
2. **Eksikse CLI'yi otomatik yükleme.** `which ocr`, “NOT INSTALLED” bildirirse ajan `npm install -g @alibaba-group/open-code-review` çalıştırıp devam eder. Bu rutin kurulum adımı için kullanıcıya soru sorulmaz.
3. **LLM yapılandırılmamışsa durup sorma.** `ocr llm test` başarısız olursa ajan kimlik bilgisi **uydurmaz**. Desteklenen iki seçeneği (ortam değişkenleri veya `ocr config set …`) gösterir ve kullanıcının API anahtarı sağlamasını bekler.
4. **İş bağlamını çıkarma.** İnceleme hedefini (commit'ler, dal, çalışma kopyası) inceleyip kısa bir `--background` metni oluşturur.
5. **İncelemeyi çalıştırma.** Kullanıcının çalışma kopyasını, belirli bir commit'i veya dal aralığını inceleme isteğine göre seçenekleri belirleyerek `ocr review --audience agent --background "…" [--commit | --from/--to]` çağırır.
6. **Sınıflandırma ve raporlama.** JSON yorumlarını SKILL.md ölçütlerine göre **High** / **Medium** / **Low** gruplarına ayırır. Hatalar ve güvenlik sorunları High düzeyindedir; küçük biçim önerileri ve olası yanlış pozitifler sessizce elenir. Ardından Markdown özeti sunulur.
7. **İstek üzerine düzeltme.** Kullanıcı “incele **ve** düzelt” gibi bir istek verdiyse High/Medium bulgularına güvenli düzeltmeler uygulanır; aksi hâlde kod değiştirilmeden önce sorulur.

Tam değerlendirme ölçütleri, çıktı şablonu ve dikkat edilmesi gereken noktaları içeren istem [`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md) dosyasındadır. Davranışı değiştirmek için (örneğin düzeltmeden önce her zaman sormasını sağlamak üzere) yerel kopyanızı düzenleyin.

## Anthropic Agent SDK {#anthropic-agent-sdk}

SDK başlangıç ayarlarında yüklü becerinin yolunu belirtin:

```python
from anthropic_agent_sdk import Agent

agent = Agent(
    skill_paths=["/path/to/open-code-review/skills/open-code-review"],
)

agent.run("Review my staged changes — focus on race conditions.")
```

SDK, SKILL.md istemini yükler; ajan [Beceri ne yapar?](#what-the-skill-does) bölümündeki iş akışını yürütür. Buna gerektiğinde `npm install` ile yükleme ve LLM yapılandırılmamışsa kimlik bilgisi isteme de dâhildir.

## Diğer ajan çatıları {#other-agent-frameworks}

Haricî beceri kaydetmeyi destekleyen her çatı SKILL.md dosyasını kullanabilir; dosya, ön bilgi başlığı içeren Markdown'dan ibarettir. Çatınız farklı bir şema beklese bile Markdown gövdesi istem şablonu olarak kullanılabilir.

## Ayrıca bakın {#see-also}

- [Komut (Claude Code eklentisi)](../claude-code/) — aynı becerinin eğik çizgi komutu biçimi.
