# Phase 10 — Güvenli Web Raporları ve Run Karşılaştırma Görünümü

## 1. Fazın Amacı

Terminal karşılaştırmasını, tamamlanmış replay artifact'leri üzerinden çalışan
görsel ve salt-okunur bir rapora dönüştürmek. Kullanıcı bir veya birden fazla
model koşusunu seçebilmeli; kalite, gecikme, token ve maliyet metriklerini
karşılaştırabilmeli; aynı `record_id` için baseline ve aday yanıtlarını güvenli
bir diff görünümünde inceleyebilmelidir.

Bu faz iki teslimat üretir:

1. **Bağımsız HTML raporu (`llm-replay report`)**: İnternet veya sunucu
   gerektirmeden açılabilen, tek dosyalık rapor.
2. **Yerel web görünümü (`llm-replay ui`)**: Büyük sonuç dosyalarında
   sayfalama ve filtreleme sağlayan, yalnızca loopback üzerinde çalışan HTTP
   sunucusu.

> **Teslim sırası:** Önce ortak artifact/karşılaştırma katmanı, sonra statik
> HTML raporu, en son yerel sunucu geliştirilir. Statik rapor tamamlanmadan UI
> sunucusuna başlanmaz.

> **Ürün sınırı:** Bu özellik replay sonuçlarını sunar; yeni replay başlatmaz,
> gözlemleme servisine veya genel amaçlı LLM dashboard'una dönüşmez.

---

## 2. Tasarım Kararları ve Ön Koşullar

### 2.1 Çoklu Run Sözleşmesi

Mevcut replay motoru her modeli ayrı `run_*` klasörüne yazar. Bu nedenle rapor
komutları tek bir run'ın çok model içerdiğini varsaymaz; bir veya daha fazla run
dizini kabul eder:

```bash
llm-replay report runs/openai-run runs/anthropic-run --output benchmark.html
llm-replay ui runs/openai-run runs/anthropic-run
```

Karşılaştırma yükleyicisi:

- Her run içindeki `config.json`, `summary.json` ve `results.jsonl` dosyalarını
  doğrular.
- Run'ların aynı `dataset_sha256` değerine sahip olmasını zorunlu tutar.
- Sonuçları dosya sırasına göre değil, `record_id` üzerinden eşleştirir.
- Bir run içinde tekrar eden `record_id` veya karşılaştırmada tekrar eden model
  kimliği varsa açık hata döndürür.
- Eksik kayıt kümelerini sessizce yok saymaz; hangi run ve kayıtların eksik
  olduğunu bildirir.
- Desteklenmeyen `schema_version`, bozuk JSONL ve tamamlanmamış run dizinlerini
  reddeder.

Bu fazda farklı dataset hash'lerine sahip run'ları zorla karşılaştıran bir
`--force` seçeneği eklenmez. Yanlış karşılaştırma üretmek, rapor üretmemekten
daha tehlikelidir.

### 2.2 Tarafsız Tradeoff Sunumu

Arayüz kullanıcı önceliklerini bilmeden tek bir “kazanan model” ilan etmez.
Bunun yerine doğrulanabilir etiketler gösterir:

- En yüksek başarı oranı
- En yüksek JSON/Schema uyumu
- En düşük ortalama ve P95 gecikme
- En düşük tahmini maliyet

Genel skor veya ağırlıklı kazanan hesabı bu fazın kapsamında değildir.

### 2.3 Maliyet Projeksiyonu

10.000, 100.000 ve 1.000.000 istek projeksiyonları yalnızca fiyat bilgisi
mevcutsa gösterilir. Görünümde bunun mevcut run'ın ortalama token kullanımına
dayalı **doğrusal bir tahmin** olduğu açıkça yazılır. Prompt caching, batch
indirimi ve uzun-context fiyatlandırması hesaba katılmıyorsa sonuç kesin fatura
tahmini olarak sunulmaz.

---

## 3. CLI ve Kullanıcı Deneyimi

### 3.1 Bağımsız HTML Raporu

```bash
llm-replay report <run_dir> [run_dir...] --output benchmark.html
```

Bayraklar:

- `--output`, `-o`: Zorunlu `.html` çıktı yolu.
- `--max-results`: HTML içine eklenecek en fazla kayıt; varsayılan `5000`.
- `--failures-only`: Yalnızca başarısız istek veya evaluator sonucu bulunan
  kayıtları dahil eder.

Kayıt sayısı `--max-results` değerini aşarsa rapor bunu hem terminalde hem HTML
içinde açıkça belirtir (`included / total`). Veri sessizce kırpılmaz.

### 3.2 Yerel Web Görünümü

```bash
llm-replay ui <run_dir> [run_dir...] [flags]
```

Bayraklar:

- `--host`: Varsayılan `127.0.0.1`; yalnızca loopback IP veya `localhost` kabul
  edilir.
- `--port`: Varsayılan `0`; işletim sistemi güvenli, boş bir port seçer.
- `--no-browser`: Tarayıcıyı otomatik açmaz, URL'yi terminale yazar.

Kullanıcı açık bir port verdiyse ve port doluysa başka porta sessiz fallback
yapılmaz; komut anlaşılır hata verir. `--port 0` kullanıldığında seçilen gerçek
URL terminale yazılır.

Run dizini verilmemesi bu fazda desteklenmez. “En son run” seçiminin dosya
zamanına göre belirsiz davranması engellenir.

---

## 4. Arayüz Kapsamı

### 4.1 Özet ve Model Karşılaştırması

- Model başına success rate, JSON validity, schema adherence ve exact match.
- Ortalama, P50 ve P95 gecikme.
- Input/output token toplamları ve oranları.
- Fiyat mevcutsa toplam ve başarılı istek başına tahmini maliyet.
- Tarafsız “en hızlı / en ucuz / en yüksek uyum” etiketleri.

P90 ve P99 gibi ek yüzdelikler `results.jsonl` içindeki istek bazlı
gecikmelerden hesaplanabilir; `summary.json` içinde varmış gibi varsayılmaz.

### 4.2 Performans Görünümü

- Model başına gecikme histogramı.
- P50/P90/P95/P99 işaretleri.
- Timeout ve uç değerlerin ayrı işaretlenmesi.
- Grafiklerin Canvas veya gömülü SVG ile, harici CDN olmadan çizilmesi.

### 4.3 İstek Gezgini

Desteklenen filtreler:

- `status`: success, error veya timeout.
- Başarısız evaluator: json, schema veya exact match.
- Minimum gecikme (`min_latency_ms`).
- `record_id` ve istek metninde basit arama.
- Model seçimi.

Her satır record ID, model, durum, gecikme, token ve evaluator özetini gösterir.
Detay görünümü baseline, candidate, normalize edilmiş hata ve evaluator
detaylarını sunar.

### 4.4 Güvenli Diff Görünümü

- MVP'de satır ve kelime bazlı diff desteklenir; karakter bazlı diff zorunlu
  değildir.
- JSON yanıtları parse edilebiliyorsa önce girintili ve kararlı biçimde
  formatlanır.
- Diff algoritması uzun girdilerde sınırsız `O(n²)` bellek kullanamaz.
- Her iki metinden biri `20.000` karakteri aşarsa otomatik diff yerine düz,
  kaçışlanmış side-by-side görünüm ve boyut uyarısı gösterilir.
- Kullanıcı içeriği hiçbir zaman `innerHTML` ile doğrudan DOM'a eklenmez.

### 4.5 Erişilebilirlik ve Tema

- Sistem temasını izleyen açık/koyu mod (`prefers-color-scheme`).
- Semantik HTML, klavye ile gezinme ve görünür focus stilleri.
- Renk tek başına başarı/hata anlamı taşımaz; metin veya ikon eşlik eder.
- Masaüstü ve dar ekranlarda kullanılabilir responsive yerleşim.

---

## 5. Teknik Mimari

```text
internal/artifact/
├── loader.go          # config, summary ve streaming JSONL doğrulama
├── query.go           # filtreleme, cursor ve record_id indeksi
└── types.go           # UI/report için provider-bağımsız view modelleri

internal/report/
├── html.go            # bağımsız HTML üretimi
└── html_test.go

internal/ui/
├── server.go          # loopback HTTP sunucusu ve API
├── browser.go         # güvenli, shell kullanmayan tarayıcı açma
├── assets.go          # //go:embed assets/*
└── assets/
    ├── index.html
    ├── app.css
    └── app.js
```

`internal/artifact` hem terminal raporları hem HTML export hem de UI tarafından
kullanılan tek doğruluk kaynağıdır. HTML exporter ve UI sunucusu kendi ayrı run
parse mantıklarını yazmaz.

Frontend varlıkları bir kez geliştirilir:

- Sunucu modunda `embed.FS` üzerinden servis edilir.
- Statik raporda aynı CSS/JS HTML içine alınır ve veri bloğu eklenir.
- Runtime sırasında Node.js, npm, CDN, harici font veya analytics kullanılmaz.
- Build/test araçları runtime bağımlılığı sayılmaz; fakat kaynak frontend için
  zorunlu bir npm build adımı eklenmez.

---

## 6. Yerel Sunucu API Sözleşmesi

API `/api/v1` altında versiyonlanır:

- `GET /api/v1/health`: Sabit sağlık yanıtı.
- `GET /api/v1/report`: Yüklenen run'ların config ve özet view model'i.
- `GET /api/v1/results`: Cursor tabanlı, filtrelenebilir sonuç listesi.
- `GET /api/v1/results/{record_id}`: Bir kaydın tüm modellerdeki detayları.

`GET /api/v1/results` parametreleri:

```text
?status=error&evaluator=schema_adherence&passed=false
&min_latency_ms=2000&model=openai/gpt-4o-mini
&limit=50&cursor=<opaque>
```

Kurallar:

- Varsayılan `limit=50`, maksimum `limit=200`.
- Negatif/geçersiz değerler `400` döndürür.
- Cursor opaque'tur; istemci dosya offset'i veya yerel path göndermez.
- API cevapları kararlı JSON tipleri ve toplam/sonraki-cursor bilgisini içerir.
- `record_id` URL-decode edildikten sonra yalnızca indekste aranır; dosya yolu
  olarak kullanılmaz.

Sunucu açılırken artifact'leri bir kez stream ederek salt-okunur, `record_id`
tabanlı bir bellek indeksi oluşturur. Her sayfa isteğinde dosyaları yeniden
taramaz. Basit ve kararlı bu yaklaşımın bellek tüketimini sınırsız bırakmamak
için seçili artifact dosyalarının toplamına `512 MiB` güvenlik sınırı uygulanır;
daha büyük karşılaştırmalar açık hata ile reddedilir.

---

## 7. Güvenlik ve Gizlilik Gereksinimleri

UI gerçek prompt ve model çıktılarını gösterdiğinden capture proxy ile aynı
güvenlik seviyesine tabidir:

1. Sunucu yalnızca doğrulanmış loopback adresine bind olur; `0.0.0.0`, LAN IP ve
   boş host reddedilir.
2. Beklenmeyen `Host` başlıkları reddedilerek DNS rebinding riski azaltılır.
3. CORS başlıkları eklenmez; API başka origin'lere açılmaz.
4. Yalnızca `GET` ve `HEAD` desteklenir; diğer metotlar `405` döndürür.
5. API'ye run path veya keyfi dosya path'i gönderilemez. Run dizinleri yalnızca
   süreç başlangıcında CLI argümanlarından alınır ve canonical path kontrolünden
   geçer.
6. Statik dosyalarda `Content-Type`, `X-Content-Type-Options: nosniff`, uygun
   `Content-Security-Policy` ve `Cache-Control` başlıkları ayarlanır.
7. Tarayıcı açma yardımcıları shell string oluşturmaz; `exec.Command` ve ayrı
   argümanlar kullanır.
8. HTML içindeki veri `json.Marshal` ile üretilir; `<`, `>`, `&`, U+2028 ve
   U+2029 kaçışları korunur. Hazır JSON dışında `template.JS` kullanılmaz.
9. `</script>`, HTML etiketi veya olay handler'ı içeren promptların kod
   çalıştıramadığı test edilir.
10. HTML raporu kullanıcıya prompt/çıktı verisi içerdiğini bildirir ve dosya
    `0600` izinleriyle oluşturulur.

Auth eklenmemesi yalnızca loopback zorunluluğu altında kabul edilir. Uzak ağda
paylaşılabilir bir UI bu fazın kapsamında değildir.

---

## 8. Büyük Veri ve Hafiflik Sınırları

- Statik HTML varsayılan olarak en fazla `5000` birleşik kayıt içerir.
- UI API tek cevapta en fazla `200` kayıt döndürür.
- Bir karşılaştırmaya yüklenen artifact dosyalarının toplamı en fazla `512 MiB`
  olabilir.
- Diff en fazla `20.000` karakterlik metinlerde hesaplanır.
- Gömülü, minify edilmemiş HTML/CSS/JS kaynaklarının toplamı `500 KiB` altında
  kalır.
- Üçüncü taraf runtime isteği, CDN veya telemetri çağrısı bulunmaz.
- Büyük JSONL dosyaları `bufio.Scanner` varsayılan limitine güvenmeden,
  projedeki güvenli satır sınırlarıyla streaming olarak okunur.

Bu sınırlar UI içinde görünür olmalı; truncation veya atlanan kayıtlar kullanıcıdan
saklanmamalıdır.

---

## 9. Uygulama Sırası

### Adım 1 — Artifact ve Karşılaştırma Katmanı

1. Run dosyaları için tipli loader ve hata taksonomisi.
2. Dataset hash, schema version, model ve duplicate ID kontrolleri.
3. `record_id` tabanlı çoklu-run join.
4. Filtreleme için ortak query/view tipleri.

### Adım 2 — Statik HTML Raporu

1. Ortak, gömülebilir frontend varlıkları.
2. Özet kartları, metrik tablosu, histogram ve istek gezgini.
3. Güvenli veri enjeksiyonu ve offline çalışma.
4. Boyut sınırı, failure-only modu ve diff fallback'i.
5. `llm-replay report` CLI entegrasyonu.

### Adım 3 — Yerel UI Sunucusu

1. Loopback server ve versiyonlu API.
2. Cursor/offset indeksi ve filtreleme.
3. Güvenlik başlıkları ve Host doğrulaması.
4. Güvenli tarayıcı açma yardımcıları.
5. `llm-replay ui` CLI entegrasyonu ve graceful shutdown.

---

## 10. Test Planı

### Artifact Testleri

- Aynı dataset üzerindeki iki run'ın ID ile doğru eşleştirilmesi.
- Farklı dataset hash, duplicate ID/model ve eksik kayıt kümelerinin reddi.
- Bozuk JSONL, eksik artifact ve desteklenmeyen schema version hataları.
- Farklı sonuç sıralarının karşılaştırmayı etkilememesi.

### HTML Rapor Testleri

- Tek ve çoklu run raporu üretimi.
- Çıktıda harici `http://` veya `https://` asset bulunmaması.
- Özet, tüm seçili modeller ve `included / total` bilgisinin gömülmesi.
- `</script><script>`, HTML etiketi ve Unicode ayırıcı içeren promptlarda XSS
  regresyon testi.
- `--failures-only` ve `--max-results` davranışı.
- Headless tarayıcıyla `file:///` üzerinden filtre ve detay etkileşimi.

### UI Sunucusu Testleri

- `httptest` ile health, report, list ve detail endpoint'leri.
- Status, evaluator, latency, model ve arama filtreleri.
- Cursor devamlılığı, maksimum limit ve geçersiz query cevapları.
- Loopback dışı bind, kötü Host, path traversal ve desteklenmeyen HTTP metodu.
- Port çakışması, `--port 0` URL çıktısı ve graceful shutdown.
- Tarayıcı helper'ının platform komutlarını shell kullanmadan üretmesi; testler
  gerçek tarayıcı açmaz.

### Diff Testleri

- Aynı, kısmen farklı, tamamen farklı ve boş metinler.
- Unicode ve çok satırlı içerik.
- JSON pretty-print ve geçersiz JSON fallback'i.
- Boyut sınırında diff'in devre dışı kalması.
- HTML özel karakterlerinin metin olarak kalması.

---

## 11. Ölçülebilir Kabul Kriterleri

- [x] `llm-replay report run-a run-b -o report.html` aynı dataset hash'ine sahip
      iki modeli `record_id` ile eşleştirerek tek, bağımsız HTML üretir.
- [x] Rapor ağ kapalıyken `file:///` üzerinden açılacak şekilde tek dosyadır;
      ortak frontend'in özet, filtreleme, detay ve diff akışı tarayıcıda doğrulanır.
- [x] Rapor hiçbir CDN, font, analytics veya harici runtime isteği içermez.
- [x] Statik rapor kayıt sınırını ve dahil edilen/toplam kayıt sayısını açıkça
      gösterir.
- [x] `llm-replay ui run-a run-b --port 0 --no-browser` yalnızca loopback'te
      başlar, gerçek URL'yi yazdırır ve Ctrl+C/SIGTERM ile temiz kapanır.
- [x] UI API sonuçları status, evaluator, latency, model ve metin filtresine göre
      doğru biçimde cursor ile sayfalar; bir cevap 200 kaydı aşmaz.
- [x] Farklı dataset, duplicate ID/model, bozuk/tamamlanmamış run ve schema
      sürümü uyuşmazlıkları anlaşılır hata üretir.
- [x] Prompt içeriği HTML/JavaScript çalıştıramaz; XSS ve path traversal testleri
      geçer.
- [x] Baseline ve her candidate yanıtı güvenli side-by-side görünümde açılır;
      sınır altındaki metinlerde diff, büyük metinlerde uyarılı fallback çalışır.
- [x] Gömülü frontend kaynakları toplam `500 KiB` altında kalır ve GoReleaser'ın
      mevcut altı hedefi ek runtime bağımlılığı olmadan derlenir.
- [x] Mevcut CLI, replay artifact formatı, testler, race detector ve smoke test
      geriye dönük olarak yeşil kalır.

---

## 12. Anti-Scope

- Kullanıcı hesabı, login, JWT, session veya uzaktan erişim.
- UI üzerinden replay başlatma, durdurma veya provider anahtarı girme.
- SaaS dashboard, bulut senkronizasyonu, telemetry veya analytics.
- SQLite/veritabanı zorunluluğu.
- Genel amaçlı run kataloğu veya geçmiş run keşfi.
- Kullanıcı tanımlı ağırlıklar olmadan genel “kazanan model” skoru.
- Streaming ve tool-call görselleştirmesi.
- Node.js/npm runtime veya zorunlu frontend build pipeline'ı.
- PDF export, ekip paylaşım sunucusu veya rapor barındırma.

`llm-replay check` ve CI regresyon eşikleri ürünün ayrı, yüksek öncelikli
otomasyon özelliğidir; bu görsel raporlama fazının içine gizlice eklenmez.
