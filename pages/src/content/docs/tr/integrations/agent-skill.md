---
title: Ajan becerisi
sidebar:
  order: 1
---

OCR'yi çağrılabilir bir beceri olarak kaydedin. Böylece ajan çatısı, doğru seçenekler ve bulgu doğrulama iş akışıyla OCR'yi çağırabilir; bunları çağıran tarafta yeniden tanımlamanız gerekmez.

## Depoda neler var? {#what-ships-in-the-repo}

Depo, [`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md) konumunda bir SKILL bildirimi içerir. OCR'yi çağırma, bulguları doğrulama, belirsizliği raporlama ve istek üzerine düzeltme adımlarını tanımlar. Beceri dizini GitHub PR inceleme yardımcısını da içerir; yalnızca SKILL.md dosyasını değil, dizinin tamamını yükleyin.

## Yükleme {#install}

### Seçenek 1: `npx skills add` (önerilen) {#option-1-npx-skills-add-recommended}

Beceriyi kullanmak istediğiniz projenin içinde çalıştırın:

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

Beceri dizini [beceri kaynağından](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md) alınarak projeye eklenir. Beceri düzenini destekleyen kodlama ajanları bir sonraki çağrıda bunu algılar. Son sürüme güncellemek için komutu yeniden çalıştırın.

> **Ön koşullar:** Yerel mod, OCR model sağlayıcısının yerel olarak yapılandırılmasını gerektirir; kimlik bilgilerini sohbete yapıştırmayın. `ocr` eksikse sorun giderme adımları `npm install -g @alibaba-group/open-code-review` ile yüklemeyi açıklar. Yalnızca GitHub PR modu ayrıca Python 3 ve kimliği doğrulanmış `gh` gerektirir. [Yapılandırma](../../configuration/) sayfasına bakın.

### Seçenek 2: Elle kopyalama (sistem genelinde) {#option-2-manual-copy-system-wide}

Proje başına yüklemek yerine genel kullanım için klasörü beceri dizininize kopyalayın:

```bash
mkdir -p ~/.claude/skills
cp -R /path/to/open-code-review/skills/open-code-review ~/.claude/skills/
```

Böylece beceri makinedeki tüm projelerde kullanılabilir.

## Beceri ne yapar? {#what-the-skill-does}

SKILL.md bir istemdir: çağıran ajan dosyayı yüklediğinde adımları kendisi yürütür. Tek bir `/open-code-review` (veya eşdeğeri) isteği şu şekilde ilerler:

1. **İş bağlamını çıkarma.** İnceleme hedefini (commit'ler, dal, çalışma kopyası) inceleyip kısa bir `--background` metni oluşturur.
2. **İstenen incelemeyi çalıştırma.** Yerel incelemelerde doğrudan `ocr review --audience agent --background "…"` çağrılır; commit edilmiş değişiklikler için `--commit` veya `--from`/`--to` eklenir. Normal akışta önce yükleme ya da bağlantı kontrolü yapılmaz. Yeni commit ve aralık incelemelerine varsayılan olarak `--analysis` eklenir; başka biçim istenmediyse JSON kullanılır. Çalışma kopyası, önizleme, devam ettirme, SARIF veya analizin açıkça istenmediği durumlarda analiz eklenmez. Başarısız çağrının nedeni araştırılmadan yeniden denenmez.
   GitHub PR numarası veya URL'si için yüklü `python3 "$SKILL_DIR/scripts/review_pr.py" <id-or-url>` yardımcısı kullanılır. Numara, mevcut çalışma kopyasının `gh` varsayılan deposunu seçer; bu, bir fork'un üst deposu olabilir. PR URL'si, incelenen revizyonlar, anlık görüntünün güncelliği ve çıktı yolu raporlanır.
3. **Doğrulama ve raporlama.** Düşük önem derecesindekiler dâhil her bulgu incelenen kodla karşılaştırılır. **Reproduced (yeniden üretildi)** ve **Uncertain (kesinleşmedi)** grupları kullanılır; OCR'nin önem derecesi ve her grup içindeki sırası korunur. Bir bulgu yalnızca somut karşı kanıt varsa çıkarılır; belirsizlik tek başına eleme nedeni değildir. Ham sonuç korunur, Review kapsamı ve eksikleri raporlanır. Birlikte çalıştırıldığında Analysis Markdown'ı ayrı sunulur; yürütme durumu ile modelin tamamlanmışlık değerlendirmesi birbirinden ayrılır.
4. **İstek üzerine düzeltme.** Yalnızca kullanıcı düzenleme istediğinde güvenli ve doğrulanmış düzeltmeler uygulanır; critical, high ve medium bulgularına öncelik verilir. Yalnızca inceleme isteği kod değiştirme yetkisi vermez. PR modunda anlık görüntü, kullanıcı açıkça temizlenmesini onaylayana kadar saklanır; temizlikten sonra raporlar korunur.

Doğrulama gerekliliklerini, raporlamayı, PR modu sınırlamalarını ve dikkat edilmesi gereken noktaları içeren istem [`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md) dosyasındadır. Davranışı değiştirmek için (örneğin düzeltmeden önce her zaman sormasını sağlamak üzere) yerel kopyanızı düzenleyin.

## Anthropic Agent SDK {#anthropic-agent-sdk}

SDK başlangıç ayarlarında yüklü becerinin yolunu belirtin:

```python
from anthropic_agent_sdk import Agent

agent = Agent(
    skill_paths=["/path/to/open-code-review/skills/open-code-review"],
)

agent.run("Review my staged changes — focus on race conditions.")
```

SDK, SKILL.md istemini yükler; ajan [Beceri ne yapar?](#what-the-skill-does) bölümündeki iş akışını yürütür. Başarısız çağrıları araştırır ve gerektiğinde kullanıcıdan kimlik bilgilerini yerel olarak yapılandırmasını ister.

## Diğer ajan çatıları {#other-agent-frameworks}

Haricî beceri kaydetmeyi destekleyen her çatı SKILL.md dosyasını kullanabilir; dosya, ön bilgi başlığı içeren Markdown'dan ibarettir. Çatınız farklı bir şema beklese bile Markdown gövdesi istem şablonu olarak kullanılabilir.

## Ayrıca bakın {#see-also}

- [Komut (Claude Code eklentisi)](../claude-code/) — aynı becerinin eğik çizgi komutu biçimi.
