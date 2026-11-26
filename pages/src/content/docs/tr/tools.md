---
title: Araçlar
sidebar:
  order: 9
---

OCR, LLM'in inceleme sırasında çağırabileceği **altı yerleşik araç** içerir. Bu sayfa her aracın amacını, girdi şemasını ve örnek girdi/çıktısını açıklar. Makine tarafından okunabilen temel tanımlar [`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json) dosyasındadır.

## Aşamalara göre araç kullanılabilirliği {#tool-availability-per-phase}

Görev şablonları, ortak tanımlardan araçları adlarıyla seçer. `task_template.json` içindeki `MAIN_TASK.tools`, çağrılabilir araçlar olarak gönderilen sıralı listedir. `PLAN_TASK.tool_references` ise planlama istemine yalnızca başvuru metni olarak eklenir. Planlama, çağrılabilir araç içermeyen tek istektir. Bu alanlar araç tanımını değiştirmez.

`scan_template.json`, ana görev araçlarını bağımsız seçer; tam dosya taramasında fark bulunmadığından `file_read_diff` eklenmez. Eksik veya boş liste hiçbir araç seçmez; tüm kataloğu etkinleştirmez. Yinelenen adlar ilk görülme sırasıyla yalnızca bir kez eklenir. Bilinmeyen başvurular, yinelenen katalog girdileri ve geçersiz tanımlar görev kurulurken hata verir.

Varsayılan fark incelemesi seçimleri:

| Araç | Plan | Ana görev | Amaç |
| --- | --- | --- | --- |
| `task_done` | ✗ | ✓ | Tamamlanmayı bildirir ve döngüyü sonlandırır. |
| `code_comment` | ✗ | ✓ | Satır aralığı ve öneri içeren inceleme yorumu üretir. |
| `file_read` | ✓ | ✓ | Seçilen karşılaştırma tarafındaki dosyanın bir bölümünü okur. |
| `file_read_diff` | ✓ | ✓ | Dosyalar arası sorunu doğrulamak için başka dosyanın farkını okur. |
| `file_find` | ✓ | ✓ | Dosyaları adlarında geçen sözcüklerle bulur. |
| `code_search` | ✓ | ✓ | Depoda düz metin veya düzenli ifadeyle arama yapar. |

Planlama salt okunur olduğundan `task_done` ve `code_comment` bu aşamada özellikle sunulmaz.

> **Bağlam araçları salt okunur bilgi sağlar; yorum hedeflerini genişletmez.** `main_task` istemi, başka dosyalardaki bulgulara yorum yapılmasını açıkça yasaklar. `file_read`, `file_read_diff`, `file_find` ve `code_search`, mevcut dosyanın farkını daha iyi anlamak içindir. Bağlam toplanırken görülen diğer sorunlar tasarım gereği yok sayılır. Dosyalar arası sorunlar yalnızca **mevcut dosyanın farkından** gözlemlenebiliyorsa yorum olur.

Yerleşik araç tanımları OCR ile gelir ve çalışma anında değiştirilemez. Proje yönlendirmesi için [inceleme kurallarını](../review-rules/), dış iş bağlamı için [MCP'yi](../mcp/) kullanın. Daha önce özel araç dosyası kullandıysanız [geçiş açıklamasına](#customizing-tools) bakın.

## Karşılaştırma tarafları {#comparison-sides}

Çalışma alanı, commit ve aralık incelemelerinde `file_read`, `code_search` ve `file_find`, isteğe bağlı `side` parametresini sunar. Parametreyi vermemek veya `"new"` vermek varsayılan davranışı ve çıktı biçimini korur. Aynı karşılaştırmanın önceki tarafını okumak, aramak veya listelemek için `"old"` kullanın:

```json
{"file_path": "src/foo.go", "start_line": 10, "end_line": 80, "side": "old"}
{"search_text": "OldAPI", "file_patterns": ["src/"], "side": "old"}
{"query_name": "foo.go", "side": "old"}
```

| İnceleme girdisi | Yeni taraf | Eski taraf |
| --- | --- | --- |
| `--from BASE --to HEAD` | Başlangıçta sabitlenen head commit'i | Sabitlenen uçların ortak atası; hedef dalın en son commit'i değil |
| `--commit COMMIT` | Sabitlenen commit | Birleştirme commit'leri dahil ilk ebeveyn |
| Kök commit | Sabitlenen commit | Boş ağaç |
| Çalışma alanı incelemesi | Canlı çalışma ağacı | Başlangıçta sabitlenen HEAD; ebeveyni değil |
| İlk commit öncesindeki çalışma alanı | Canlı çalışma ağacı | Boş ağaç |

Commit ve aralık incelemelerinde fark ile üç araç da aynı değişmez uçları kullanır. Yol ve satır numaraları seçilen tarafa aittir. Yeniden adlandırılmış dosyada eski tarafta eski yolu kullanın; araçlar adları dönüştürmez veya başka sürüme geçmez. Silinen dosyalar eski tarafta erişilebilir. Kök commit'in eski tarafında dosya yoktur: okumalar eksik dosya bildirir, aramalar ve listelemeler boş döner. Erişilemeyen Git nesneleri ve Git hataları boş ağaç göstergesi değil, hatadır.

Sığ klon sınırındaki commit'ler, ebeveyn nesneleri yerelde bulunsa bile incelemeden önce derinleştirilmelidir; böylece Git farkı ve eski taraf araçları aynı tabanı kullanır.

Çalışma alanı incelemesinde yalnızca eski taraf sabitlenir. HEAD sonradan değişse de fark ve eski taraf araçları başlangıçtaki HEAD'i kullanır. Yeni taraf, seçim kurallarına uyan izlenmeyen dosyalar dahil canlı çalışma ağacını okur; indeks veya değişmez çalışma alanı görüntüsü değildir. Tutarlı yeni taraf bağlamı gerekiyorsa inceleme sırasında dosyaları düzenlemeyin. HEAD'in ebeveyni gerekmediğinden okunabilir HEAD içeren sığ klon desteklenir.

Önizleme ve inceleme aynı çalışma alanı karşılaştırmasını kullanır. İndekse eklemeden sonra yapılan düzenlemelerde çalışma ağacındaki içerik karşılaştırılır. Boş farkta indeksteki değişikliklere geri dönülmez. İlk commit'i olmayan depo, eski indeks sürümü yerine boş ağaçla çalışma ağacını karşılaştırır. Bu, önceki indekse geri dönme davranışını düzeltir. Diğer Git hataları hata olarak kalır.

Çalışma alanında devam ettirme, yalnızca HEAD ve inceleme girdisi değişmemişse tamamlanan kontrol noktalarını kullanır. İzlenen dosyaların silinmesi/geri getirilmesi dahil seçilen fark, önceki çalıştırmayla tam eşleşmelidir. Tamamlanan dosyalar yeniden kullanılır, bitmeyenler tekrar denenir. Düzenleme sonrası kısmi yeniden kullanım ve inceleme sırasındaki düzenlemelere karşı koruma kapsam dışıdır. Karşılaştırma normalleştirilmiş fark üzerinden yapılır: izlenmeyen dosyanın son satır sonunu eklemek/kaldırmak veya yalnızca LF/CRLF değiştirmek devam etmeyi engellemez; içerik değişikliği engeller.

Eski taraf sonuçları `SIDE: old` ve `REVISION: <commit-sha>` ile; boş ağaçta `SOURCE: empty-tree` ile başlar. Hatalar da kaynaklarını belirtir. Mevcut satır ve eşleşme sınırları iki tarafa da uygulanır. Yalnızca `"new"` ve `"old"` dizeleri geçerlidir; null, boş dize ve rastgele ref değerleri geçersizdir.

Taramalar mevcut tanımlarını, istemlerini ve varsayılan çıktı biçimlerini korur; `side` sunmaz ve verilirse reddeder. Çalışma alanı incelemeleri varsayılan çıktı biçimini koruyarak isteğe bağlı taraf parametresini ekler. Ortak `tools.json` kataloğu yalnızca güncel içeriğe yönelik temel tanımları içerir; `side` tanımlamaz. Kurulumda karşılaştırmalı incelemeler `WithComparisonSides` ile bu katalogdan çağrılabilir tanımlar ve planlama başvuruları türetir, temel tanımları değiştirmeden `side` ve açıklamaları ekler. Çalışma alanı incelemeleri, sabit eski tarafı ve canlı yeni tarafı açıklamak için `WithWorkspaceSides` kullanır.

Commit/aralık incelemesine devam etmek, güncel yerleşik araçları kullanır ve seçili yamalar değişmemiş olsa bile aynı çözümlenmiş eski/yeni uçları gerektirir; ağaçların diğer bölümlerindeki bağlam önceki incelemeyi etkilemiş olabilir. Aynı uçlara çözümlenen farklı ref adları geçerlidir. Uçlardan biri değişmişse veya eski oturumda karşılaştırmayı doğrulayacak yeterli bilgi yoksa yeni inceleme başlatın. Tamamlanan dosyalar yeniden kullanılır, bekleyenler güncel araçlarla incelenir. Tüm dosyaları güncel yeteneklerle yeniden kontrol etmek için yeni inceleme başlatın. Çalışma alanında da aynı kural geçerlidir; kimlik commit çifti yerine HEAD ve inceleme girdisidir. İzlenen dosyanın silinmesi/geri getirilmesi dahil her girdi değişikliği yeni inceleme gerektirir.

## `task_done` {#taskdone}

Ana döngüyü sonlandırır.

```json
{
  "name": "task_done",
  "input": { "state": "DONE" }
}
```

| Alan | Zorunlu | Anlamı |
| --- | --- | --- |
| `state` | Evet | `DONE` (varsayılan) veya `FAILED`. `FAILED`, mevcut araçlarla görevin gerçekten yapılamadığı durum içindir; neredeyse hiçbir zaman doğru seçim değildir. |

Ajan `task_done` gördüğünde LLM çağrılarını durdurur ve biriken `code_comment` çağrılarını işlemeye başlar. Sonuç oturum günlüğüne kaydedilmeden hemen döndüğünden `state` kabul edilir ancak **kalıcı kayda yazılmaz**; çıkış kodunu da etkilemez.

## `code_comment` {#codecomment}

Bir veya daha fazla inceleme yorumu üretir. OCR'ın satırları otomatik hesaplayabilmesi için her yorum bir kod parçasına (`existing_code`) bağlanır.

### Şema {#schema}

```json
{
  "name": "code_comment",
  "input": {
    "path": "string — optional, override the file path for this comment",
    "comments": [
      {
        "content": "string — the comment in the configured language",
        "existing_code": "string — snippet from the diff to anchor on",
        "suggestion_code": "string — optional fix snippet",
        "thinking": "string — optional, the model's reasoning for this comment"
      }
    ]
  }
}
```

`comments` dizi olduğundan model tek çağrıda birden fazla yorum üretebilir. `content` ve `existing_code` zorunludur. `suggestion_code` isteğe bağlıdır ancak önerilir. Üst düzey `path` isteğe bağlıdır; verilmezse ajan incelenen dosyanın yolunu ekler, bu nedenle modelin genellikle açıkça belirtmesi gerekmez. Yorum başına `thinking`, modelin gerekçesini saklar. OCR bunu mevcut turda modelin ürettiği akıl yürütme içeriğinden tamamlar; model üretmezse boş kalır. JSON çıktısına eklenir, terminal çıktısında gösterilmez.

> **`thinking` yalnızca çalışma anında kullanılan bir alandır.** OCR ayrıştırıp saklar ancak `tools.json` içinde modele sunulan `code_comment` şemasında özellikle yer almaz; yalnızca `content`, `existing_code` ve `suggestion_code` sunulur. Yine de `thinking` üreten modellerin içeriği saklanır. Çoğu model bunu göndermez; bu normaldir.

### Konumlandırma algoritması {#anchoring-algorithm}

OCR, `existing_code` metnini **dinamik kayan pencere** ile farkta arar. Sırasıyla:

1. **Parçanın yeni tarafı** — Art arda gelen **bağlam + eklenen** satırlar aranır; yalnızca silinmiş veya yalnızca değişmemiş satırlar kullanılmaz. Yeni dosyanın satır numaraları üretilir. Bulunamazsa **eski taraf**, yani bağlam + silinen satırlar denenir ve eski dosya numaraları üretilir.
2. **Yeni dosyanın tamamını tarama** — Parçada eşleşme yoksa değişiklik sonrası dosyanın tamamı art arda eşleşen satırlar için taranır (`resolveFromFileContent`).
3. **Yeniden konumlandırma görevi** — Basit olmayan farkta metin eşleştirme hâlâ başarısızsa `RE_LOCATION_TASK`, modelden parçayı yeniden konumlandırmasını ister.

Eşleştirme **boşluklara duyarsızdır**: satır uçları kırpılır ve farkın `+`/`-` işaretleri kaldırılır; girintinin tam eşleşmesi gerekmez. Son çare olarak `start_line=0` döndürülür; bu, yorumun konumunun kullanıcı tarafından bulunması gerektiğini belirtir.

### Örnek {#example}

```json
{
  "comments": [
    {
      "content": "`tx.Rollback()` is never deferred — early returns leak the transaction.",
      "existing_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}",
      "suggestion_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}\ndefer tx.Rollback()"
    }
  ]
}
```

## `file_read` {#fileread}

Varsayılan olarak dosyanın **değişiklik sonrası** biçiminden bir satır aralığı okur. Çalışma alanı, commit ve aralık incelemeleri isteğe bağlı [karşılaştırma tarafını](#comparison-sides) da destekler.

### Şema {#schema-2}

```json
{
  "name": "file_read",
  "input": {
    "file_path": "src/foo.go",
    "start_line": 10,
    "end_line": 80
  }
}
```

| Alan | Zorunlu | Varsayılan | Açıklama |
| --- | --- | --- | --- |
| `file_path` | Evet | — | Depo köküne göre yol. |
| `start_line` | Hayır | `1` | Numaralandırma 1'den başlar. |
| `end_line` | Hayır | Dosya sonu | Son satır dahildir. |

### Çıktı {#output}

```
File: src/foo.go (Total lines: 220)
IS_TRUNCATED: false
LINE_RANGE: 10-80
10|package foo
11|
12|import (
13|    "fmt"
…
```

Her içerik satırının başında 1 tabanlı satır numarası ve `|` ayırıcı bulunur; model sonraki `code_comment` çağrılarında kesin satır numarası verebilir.

### Sınırlar {#limits}

- **Çağrı başına en fazla 500 satır.** Büyük aralıklar kısaltılır, `IS_TRUNCATED: true` ayarlanır ve `Note: Results truncated to 500 lines. Please narrow your line range.` notu eklenir.
- **Bayt sınırları.** 2.048 bayttan uzun satır UTF-8 karakter sınırında kesilir; `[line truncated, N more bytes omitted]` ile işaretlenir. Gövde 256 KiB'a ulaştığında da çıktı durur ve `Note: Output truncated at 262144 bytes. …` eklenir. İkisinde de `IS_TRUNCATED: true` olur; böylece küçültülmüş tek satırlı dosyalar konuşmaya megabaytlarca veri gönderemez.
- Varsayılan olarak **değiştirilmiş sürümü** okur. Karşılaştırmalı incelemede eski dosyanın tamamı için `side: "old"`, yama için `file_read_diff` kullanın.

Model çevre bağlama ihtiyaç duyduğunda (ör. yalnızca farkta gördüğü işlev için), aralığı `@@ -x,y +m,n @@` başlığından hesaplamalıdır; genellikle `m-50` ile `m+n+50` arası uygundur.

## `file_read_diff` {#filereaddiff}

Aynı değişiklik kümesindeki bir veya daha fazla **diğer** dosyanın farkını okur. Yorum, ilişkili dosyanın güncellenip güncellenmediğine bağlıysa yararlıdır.

### Şema {#schema-3}

```json
{
  "name": "file_read_diff",
  "input": {
    "path_array": ["src/api/handler.go", "src/db/queries.go"]
  }
}
```

### Çıktı {#output-2}

```
==== FILE: src/api/handler.go ====
--- a/src/api/handler.go
+++ b/src/api/handler.go
@@ -10,1 +10,2 @@
- old line
+ new line 1
+ new line 2

==== FILE: src/db/queries.go ====
@@ -5,1 +5,1 @@
- query := "SELECT *"
+ query := "SELECT id"
```

Değişiklik kümesinde bulunmayan yol sessizce atlanmaz; `Error: diff not found for: <path>` ile belirtilir. Bulunan yolların farkları önce döndürülür. Başka farklar varsa `Diffs exist for: ...` ile listelenir; yanlış yazılmış yol düzeltilebilir. Hiç fark yoksa `file_read` önerilir. Boş `path_array`, `Error: no files found` döndürür.

## `file_find` {#filefind}

Depoda göreli yol veya dosya adının bir bölümüne göre dosya bulur; alt dize eşleştirmesi kullanır.

### Şema {#schema-4}

```json
{
  "name": "file_find",
  "input": {
    "query_name": "UserService",
    "case_sensitive": false
  }
}
```

| Alan | Zorunlu | Varsayılan | Açıklama |
| --- | --- | --- | --- |
| `query_name` | Evet | — | Her dosyanın **depoya göre yolunda** aranan alt dize; `/` ve `\` ayırıcıları kabul edilir. |
| `case_sensitive` | Hayır | `false` | Büyük/küçük harfe duyarlı eşleştirme için `true`. |

Adaylar çalışma alanı kipinde `git ls-files --cached --others --exclude-standard`, aralık/commit kipinde `git ls-tree -r --name-only <ref>` ile alınır. `Makefile`, `Dockerfile`, `LICENSE`, `Vagrantfile`, `Containerfile`, `BUILD` ve `WORKSPACE` dışındaki uzantısız dosyalar atlanır.

### Çıktı {#output-3}

Her satırda bir yol bulunur:

```
src/main/java/com/example/UserService.java
src/test/java/com/example/UserServiceTest.java
src/main/java/com/example/internal/UserServiceImpl.java
```

Eşleşme yoksa veya `query_name` boşsa aynen `// The file was not found` döndürülür.

### Sınırlar {#limits-2}

En fazla **100** eşleşme döndürülür; fazlası sessizce kesilir. Daha geniş arama gerekiyorsa model `code_search` kullanmalıdır.

## `code_search` {#codesearch}

Depoda tam metin araması yapar. `git grep` kullandığı için `pathspec` sözdizimini anlar ve `.gitignore` kurallarına uyar.

### Şema {#schema-5}

```json
{
  "name": "code_search",
  "input": {
    "search_text": "TODO|FIXME",
    "file_patterns": ["*.go", ":(exclude)vendor/"],
    "case_sensitive": false,
    "use_perl_regexp": true
  }
}
```

| Alan | Zorunlu | Varsayılan | Açıklama |
| --- | --- | --- | --- |
| `search_text` | Evet | — | Düz dize veya PCRE kalıbı; bkz. `use_perl_regexp`. |
| `file_patterns` | Hayır | Tüm depo | Pathspec girdileri dizisi; dışlamak için `:(exclude)pat`. |
| `case_sensitive` | Hayır | `false` | — |
| `use_perl_regexp` | Hayır | `false` | `true` ise `search_text` düzenli ifade olarak yorumlanır. |

Unicode kod noktaları için `\x{00A0}` gibi PCRE2 kaçış dizileri kullanın. JavaScript biçimindeki `\u00A0` desteklenmez.

### Çıktı {#output-4}

Sonuçlar dosyaya göre gruplanır. Her grup `File: <path>` ve `Match lines: <n>` ile başlar; her eşleşme `line|content` biçiminde yazılır:

```
File: path/to/example.java
Match lines: 2
433|      String name = toolRequest.get().getName();
438|      logToolRequest(newPath, tool, toolRequest.get());

File: path/to/other.java
Match lines: 1
22|      var req = new ToolRequest();
```

Eşleşme yoksa aynen `No matches found` döndürülür.

### Pathspec örnekleri {#pathspec-cookbook}

| Amaç | `file_patterns` |
| --- | --- |
| Tek dosya | `["src/main.go"]` |
| Tüm Go dosyaları | `["*.go"]` |
| Testler dışındaki Go dosyaları | `["*.go", ":(exclude)*_test.go"]` |
| Tek dizin | `["src/api/"]` |
| Birden fazla tür, bağımlılık dizinleri hariç | `["*.go", "*.ts", ":(exclude)vendor/", ":(exclude)node_modules/"]` |

### Sınırlar {#limits-3}

- `git grep --max-count 100` ile **dosya başına 100** eşleşme sınırı vardır; birçok dosyanın toplam çıktısı 100'ü aşabilir. Sınıra ulaşıldığında başa `Note: The results have been truncated. Only showing first 100 results.` eklenir.
- Boş veya yalnızca boşluk içeren `search_text`, tüm satırlarla eşleşmek yerine `Error: search_text is blank` döndürür.
- Çalışma alanı kipinde **canlı çalışma ağacını**, aralık/commit kipinde çözümlenmiş hedef ref'i arar (`FileReader.Ref`, `git grep` komutuna konumsal argüman olarak geçilir).

## Araç yürütme ve hatalar {#tool-execution-errors}

İki istisna dışında araçlar ajan döngüsü içinde senkron çalışır:

- `code_comment`, satır belirleme ve değerlendirme nedeniyle döngüyü bekletmemek için **CommentWorkerPool** havuzuna gönderilir.
- `task_done`, sağlayıcı çağırmadan hemen döner.

Ağ hatası, geçersiz argüman veya bulunamayan dosya gibi hatalar modele normal araç sonucu olarak iletilir; örneğin `"Error: file not found: src/missing.go"`. Model tekrar denemeye, başka dosya istemeye veya `task_done` çağırmaya karar verir.

Araç adı kayıtlı değilse OCR çökmek yerine `tool.NotAvailableMsg` sabitini döndürür; bilinmeyen çağrı modele kullanılamayan araç olarak bildirilir.

## Araç özelleştirme sınırları ve geçiş {#customizing-tools}

OCR, çekirdek iş akışı ve yerleşik araçları testler ve kıyaslamalarla birlikte doğrulanan özel bir inceleme ajanı kullanır. Kullanıcının tanımladığı bir araç kayıt sistemi kullanmaz.

**Geçiş:** `--tools`, hem `ocr review` hem `ocr scan` komutundan kaldırılmıştır. Eski çağrılar `unknown flag: --tools` hatası verir. Seçeneği ve dosya argümanını betiklerinizden, CI işlerinden ve kabuk takma adlarından kaldırın.

- Projeye özgü yönlendirme için [inceleme kuralları](../review-rules/) veya `--background` kullanın; `review` ayrıca `--background-file` destekler.
- Gereksinimler, bilgi tabanları ve diğer dış iş bağlamları için [MCP sunucularını](../mcp/) yapılandırın. MCP şu anda yalnızca `review` içinde bağlıdır; `scan` içinde yoktur ve yerleşik araç adlarının yerini alamaz.
- Yerleşik araçları kapatmanın veya açıklamalarını, şemalarını ya da aşamalara göre kullanılabilirliğini değiştirmenin çalışma anında bir karşılığı yoktur. Çekirdek değişiklikleri kaynak düzenlemesi, yeniden derleme, uygun test ve kıyaslama doğrulaması gerektirir.

`--max-tools`, araç turu bütçesini denetlemeye devam eder. `mcp_servers.<name>.tools`, dış araçları filtreler; kaldırılan seçenekle ilişkili değildir. Eski oturum dosyaları okunabilir, ancak özel araç tanımları kullanan oturuma devam etmek yerine yeni inceleme başlatın.

## Ayrıca bkz. {#see-also}

- [Mimari](../architecture/) — Ajan döngüsünün araçları kullanımı.
- [İnceleme kuralları](../review-rules/) — LLM'in odaklanacağı konular.
- [Oturum görüntüleyicisi](../viewer/) — Önceki incelemelerde çağrılan araçları görme.
