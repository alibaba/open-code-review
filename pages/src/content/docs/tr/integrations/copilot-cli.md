---
title: GitHub Copilot CLI
sidebar:
  order: 3
---

Diğer kodlama ajanlarının da kullandığı taşınabilir [Ajan becerisi](../agent-skill/) ile OCR'yi [GitHub Copilot CLI](https://docs.github.com/en/copilot/concepts/agents/about-copilot-cli) içinde uçtan uca çalıştırın: farkı inceleyin, bulguları sınıflandırın ve değerli olanları düzeltin. Entegrasyon, kopyalanıp kullanılabilen bir beceridir; kanca kurmaz, zorunlu talimat eklemez ve depo davranışını değiştirmez.

## Depoda neler var? {#what-ships-in-the-repo}

İş akışı ajandan bağımsız [`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md) becerisindedir. [`plugins/open-code-review/plugin.json`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/plugin.json) konumundaki Copilot CLI eklenti bildirimi (Agent Plugins 1.0), becerileri `copilot plugin install` için paketler. Copilot CLI standart konumlardan beceri okumayı sürdürdüğü için beceri dizinini kopyalamak da çalışır.

## Yükleme {#install}

Eklentiyi yükleyin (önerilen):

```bash
copilot plugin install alibaba/open-code-review:plugins/open-code-review
```

Bu işlem `open-code-review` ve `open-code-review-delegate` becerilerini yükler. Kaldırmak için `copilot plugin uninstall open-code-review` kullanın.

Copilot CLI yükleyicisi şu anda doğrudan eklenti yüklemelerinin (depo/yol/URL) mağaza yüklemeleri lehine kullanımdan kaldırıldığını bildirir. İleride bir mağaza kaydıyla `plugin@open-code-review` önerilen biçim olabilir.

### Elle kurulum alternatifi: beceriyi kopyalama {#manual-alternative-copy-the-skill}

Beceriyi Copilot CLI'nin keşfettiği bir dizine kopyalayın. **Proje düzeyinde** (ekibin paylaşması için commit'e ekleyin):

```bash
mkdir -p .github/skills
curl -fsSL https://github.com/alibaba/open-code-review/archive/refs/heads/main.tar.gz |
  tar -xz -C .github/skills --strip-components=2 open-code-review-main/skills/open-code-review
```

**Kullanıcı düzeyinde** (makinedeki tüm projelerde):

```bash
mkdir -p ~/.copilot/skills
curl -fsSL https://github.com/alibaba/open-code-review/archive/refs/heads/main.tar.gz |
  tar -xz -C ~/.copilot/skills --strip-components=2 open-code-review-main/skills/open-code-review
```

Copilot CLI, projedeki `.agents/skills/` ve `.claude/skills/` dizinlerini de keşfeder. Aynı adlı kişisel beceriye göre proje becerisi önceliklidir. Oturum içinde `/skills list` (dışında `copilot skill list`) ile yüklenenleri kontrol edin. Elle kopyalanan beceriler `/skills reload` veya yeni oturumla etkinleşir. Yüklü eklentiler yeniden yüklenerek (`copilot plugin install`) veya `copilot plugin update` ile güncellenir.

## Kullanım {#use}

Beceriyi `/open-code-review` ile açıkça belirtin veya görevi anlatın; Copilot açıklamasından beceriyi seçer:

```
/open-code-review
/open-code-review 42
/open-code-review focus on race conditions in commit abc123
```

| İsteğiniz | Becerinin çalıştırdığı komut |
| --- | --- |
| “değişikliklerimi incele” | `ocr review --audience agent` (hazırlanmış, hazırlanmamış, izlenmeyen) |
| `42` | `python3 "$SKILL_DIR/scripts/review_pr.py" 42` |
| “abc123 commit'ini incele” | `ocr review --commit abc123` |

Beceri, `ocr` aracını kendi yapılandırılmış modeliyle çalıştırır ve bulguları önem derecesine göre gruplandırır. `ocr` için ayrı LLM yapılandırmak istemiyorsanız [görev devri modunu](../delegate/) kullanın: modeli ana ajan (Copilot) sağlar; `ocr` yalnızca dosyaları seçer ve kuralları çözümler.

### İsteğe bağlı: incelemeyi alışkanlık hâline getirme {#optional-make-review-a-habit}

İncelemeyi istek üzerine değil otomatik çalıştırmak için `.github/copilot-instructions.md` veya `AGENTS.md` dosyasına tek bir kural ekleyin. Copilot talimat dosyalarını öncelik sırası olmadan birleştirdiğinden yalnızca bir dosya seçin:

```markdown
Before reporting a coding task complete, run `ocr review --audience agent`.
Fix all `critical` and `high` findings first.
```

Böyle bir kural ekleme kararı her projeye aittir; bu depo zorunlu tutmaz.

## Etkileşimsiz çalıştırma {#headless-runs}

```bash
copilot -p "Review the current branch with /open-code-review and report the findings." \
  --allow-tool='shell(ocr:*)'
```

- `--allow-tool='shell(ocr:*)'` yalnızca `ocr` kabuk komutlarına ön onay verir; dosya düzenlemelerini yetkilendirmez. Etkileşimsiz (`-p`) oturumlarda onay gereken işlemler bekletilmek yerine reddedilir; etkileşimli oturumlarda onay beklenir. Copilot'un düzeltme uygulaması için etkileşimli çalıştırın veya izin listesini bilinçli olarak düzenleme araçlarını kapsayacak şekilde genişletin.
- Güvenilmeyen klasörlerdeki etkileşimsiz oturumlarda proje becerileri (`.github/skills/`) atlanabilir. Copilot CLI v1.0.107+, klasöre tek oturum için güvenmek üzere `--trust-folder` sunar.
- Copilot yalıtılmış ortamı etkinse `ocr`, LLM uç noktasına ağ erişimine ve oturum durumunu sakladığı `~/.opencodereview/` dizinine okuma/yazma erişimine ihtiyaç duyar.
- `--output-format json`, Copilot'un JSONL oturum olaylarını verir. İnceleme sonucu ise açıkça `ocr review --format json --audience agent` çalıştırılarak alınan `ocr` JSON'udur.

Deterministik CI kontrolleri için ajanı atlayıp `ocr` aracını doğrudan çağırın; [CI/CD](../cicd/) sayfasına bakın.

## Notlar {#notes}

- **Kancalar bilinçli olarak eklenmedi.** Copilot CLI, `preToolUse` kancalarıyla `git commit` veya `git push` işlemlerini koşula bağlayabilir. Ancak OCR böyle bir kanca sunmaz: inceleme LLM çalıştırır (yavaştır ve katkıcının sahip olmayabileceği kimlik bilgileri gerekir), incelenen çalışma alanı oluşturulacak commit ile aynı değildir ve hata durumunda işleme izin veren bir kanca garanti edemediği bir zorunluluk izlenimi yaratır. Bu ödünleşimleri kabul ediyorsanız kendi kancanızı ekleyin.
- **İstem dosyaları yalnızca IDE içindir.** Copilot CLI `.github/prompts/*.prompt.md` dosyalarını okumaz; desteklenen yöntem beceridir.
- **Bulgular çıkış kodunu değiştirmez.** Sorun bulsa bile tamamlanmış incelemede `ocr` 0 koduyla çıkar. Geçiş koşulunu çıkış koduna değil, `--format json` içindeki önem derecelerine bağlayın.

## Sorun giderme {#troubleshooting}

- **Beceri listelenmiyor** — yolu (`~/.copilot/skills/` veya `<repo>/.github/skills/`) doğrulayın; ardından `/skills reload` çalıştırın veya yeni oturum açın.
- **`ocr: command not found`** — `npm i -g @alibaba-group/open-code-review` ile yükleyin.
- **İnceleme LLM hatasıyla başarısız** — sağlayıcı yapılandırıp `ocr llm test` ile sınayın; [Yapılandırma](../configuration/) sayfasına bakın.
- **Copilot sürekli `ocr` onayı istiyor** — `--allow-tool` ile `shell(ocr:*)` izni verin veya istemde kalıcı olarak onaylayın.
