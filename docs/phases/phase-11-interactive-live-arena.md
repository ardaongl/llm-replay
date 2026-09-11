# Phase 11 — Güvenli Canlı Model Arenası

## 1. Amaç ve Ürün Sınırı

Geliştiricinin aynı metin tabanlı isteği iki izinli modele paralel göndermesini;
yanıtları, gecikmeyi, token kullanımını, tahmini maliyeti ve evaluator
sonuçlarını yan yana incelemesini sağlayan yerel bir çalışma alanı sunmak.

Phase 11, iki ayrı teslimata bölünür:

1. **Phase 11A — Read-only Live Arena:** Model çağrısı, metrikler,
   değerlendirmeler ve güvenli diff.
2. **Phase 11B — Opt-in Dataset Capture:** Yalnızca CLI başlangıcında açıkça
   seçilmiş tek bir dataset'e, kullanıcı seçimiyle kayıt ekleme.

Phase 10'daki `llm-replay ui` tamamlanmış artifact'leri gösteren salt-okunur
bir rapor sunucusu olarak kalır. Provider çağrısı veya dosya yazma yetkisi
kazanmaz. Canlı Arena ayrı bir komuttur:

```bash
llm-replay arena
```

> **Phase 11C güncellemesi:** İlk tasarımdaki CLI `--model` seçimi,
> [Phase 11C](phase-11c-web-model-selection.md) ile server-allowlisted web
> dropdown'larına taşındı. Aşağıdaki güvenlik ve kayıt sözleşmeleri korunur.

> **Veri aktarımı uyarısı:** Kontrol katmanı, UI ve kayıtlar yereldir; ancak
> kullanıcının Arena'ya yazdığı mesajlar doğrudan seçilen model sağlayıcılarına
> gönderilir. “Local-first”, promptların sağlayıcıya hiç gitmediği anlamına
> gelmez. Bu gerçek arayüzde çalıştırma düğmesinin yanında açıkça gösterilir.

> **Ürün sınırı:** Arena hızlı ve tekil keşif içindir. Batch replay, CI
> regresyon eşikleri veya genel amaçlı bir observability platformunun yerini
> almaz.

Yol haritasındaki `llm-replay check` ve CI regresyon eşikleri projenin temel
“regresyon testi” değer önerisi açısından daha yüksek ürün önceliğini korur.
Arena vitrin ve keşif özelliğidir; CI özelliğini geciktirecek şekilde çekirdek
mimarinin yönünü değiştirmemelidir.

---

## 2. Neden Ayrı `arena` Komutu?

- Phase 10 sunucusunun yalnızca `GET`/`HEAD` ve salt-okunur güvenlik sözleşmesi
  değişmez.
- Harici API çağrısının maliyet ve veri aktarımı etkisi komut adından anlaşılır.
- Dataset yazma yetkisi yalnızca `--dataset` verilirse etkinleşir.
- Arena başlatmak için önceden oluşturulmuş bir run dizini gerekmez.
- Model allowlist'i binary içindeki server kataloğundan kurulur; tarayıcı keyfi
  model veya provider giremez.

`llm-replay arena` model argümanı istemez. Kullanıcı iki farklı modeli web
arayüzündeki katalogdan seçer. Bu fazda desteklenen provider'lar OpenAI ve
Anthropic'tir. Model kataloğunu provider API'lerinden otomatik keşfetme kapsam
dışıdır.

### CLI Sözleşmesi

```text
llm-replay arena [flags]

--host               Loopback host; varsayılan 127.0.0.1
--port               Yerel port; varsayılan 0
--timeout            Model çağrısı üst sınırı; varsayılan 30s, en fazla 60s
--pricing            Opsiyonel fiyat override YAML dosyası
--dataset            Opsiyonel, mevcut ve geçerli tek bir JSONL dataset
--no-browser         Tarayıcıyı otomatik açmaz
--openai-base-url    Test ve OpenAI-uyumlu yerel endpoint override'ı
--anthropic-base-url Test endpoint override'ı
```

`--dataset` verilmezse “Save to Dataset” özelliği backend'de kapalıdır ve
arayüzde gösterilmez. Tarayıcı hiçbir zaman dataset path göndermez.

Eksik provider anahtarı Arena sunucusunun açılmasını engellemez. İlgili model
`available: false` görünür, compare düğmesi devre dışı kalır ve backend yine de
çağrı yapılmasını `409 Conflict` ile reddeder. Hiçbir provider çağrısı kısmi
başlamaz ve kullanıcı farkında olmadan tek modele ücret ödemez.

---

## 3. Kullanıcı Akışı

### 3.1 Phase 11A — Karşılaştırma

1. Kullanıcı sistem ve kullanıcı mesajını girer.
2. Kullanıcı server kataloğundaki iki farklı modeli dropdown'lardan seçer.
3. Kullanıcı sıcaklık, maksimum çıktı tokenı ve opsiyonel JSON Schema belirler.
4. Arayüz promptların iki sağlayıcıya gönderileceğini ve maliyet
   oluşturabileceğini belirtir.
5. Kullanıcı “Compare models” düğmesine basar.
6. Backend iki provider çağrısını paralel başlatır ve ikisinin tamamlanmasını
   bekler.
7. Sonuçlar yan yana gösterilir. Bir çağrı başarısız olsa bile diğer sonuç
   korunur.
8. Baseline olmadan iki candidate arasındaki kelime diff'i gösterilir.

Bu fazda SSE/token streaming yoktur. “Live”, kullanıcının interaktif olarak
tek istek çalıştırabilmesini ifade eder. Tek HTTP cevabı iki çağrı da
tamamlandığında döner; “ilk model döner dönmez ekranda belirir” sözü verilmez.

### 3.2 Phase 11B — Dataset'e Kayıt

Bu akış yalnızca `--dataset` ile etkinleşir:

1. Kullanıcı başarılı Arena sonuçlarından birini **baseline response** olarak
   seçer.
2. Opsiyonel olarak:
   - Seçilen response'u exact-match için `evaluation.expected` yapar.
   - Compare isteğinde kullanılan schema'yı `evaluation.schema` olarak saklar.
3. UI, hangi provider/model cevabının kaydedileceğini açıkça özetler.
4. Kullanıcının “Save selected baseline” eyleminden sonra backend kaydı ekler.

Kaydedilen `domain.Record` şu anlamı taşır:

- `request`: Arena'da provider'lara gönderilen normalize istek.
- `provider` ve `model`: Kullanıcının baseline seçtiği başarılı sonuç.
- `response` ve `metrics`: Seçilen server-side sonuç.
- `evaluation.schema`: Kullanıcının compare sırasında verdiği opsiyonel schema.
- `evaluation.expected`: Yalnızca kullanıcı açıkça exact-match baseline'ı
  oluşturmayı seçerse seçilen response içeriği.

Başarısız/timeout sonucu baseline olarak kaydedilemez. Aynı Arena sonucu için
save isteği idempotent davranır ve ikinci bir kayıt üretmez.
Provider'ın ham HTTP cevabı (`Response.Raw`) ne tarayıcıya gönderilir ne de
dataset baseline'ına yazılır; yalnızca normalize content ve finish reason
kullanılır.

---

## 4. Backend Mimarisi

```text
cmd/llm-replay
└── arena command
    ├── embedded model allowlist + provider adapter factory
    ├── pricing registry
    ├── optional pre-opened dataset writer
    └── internal/arena.Server
        ├── Runner
        │   ├── provider.Provider (model A)
        │   └── provider.Provider (model B)
        ├── evaluator pipeline
        ├── bounded recent-result store
        └── embedded Arena frontend
```

### 4.1 `internal/arena.Runner`

Runner doğrudan provider adı/model string'i oluşturmaz. CLI katmanında kurulan
ve model ID ile eşlenen katalog spec'lerini alır. Credential mevcutsa spec bir
`provider.Provider` içerir; değilse unavailable durumunu ve kullanıcıya güvenli
hata mesajını taşır.

- Çağrılar iki goroutine ile paralel yürür.
- Her goroutine ayrı timeout context'i kullanır.
- Sonuç kanalı model sırasından bağımsızdır; response web'deki Model A / Model B
  sırasına göre kararlı biçimde düzenlenir.
- Arena otomatik retry yapmaz. Böylece tek tıklamanın kaç ücretli çağrı
  oluşturduğu öngörülebilir kalır.
- Bir model hatası bütün isteği hata yapmaz; doğrulanmış compare isteği
  `200 OK` ve model başına `success/error/timeout` sonucu döndürür.
- Client bağlantısı kapanırsa provider context'leri iptal edilir.
- UI'a yalnızca normalize hata türü, HTTP status ve güvenli kısa mesaj verilir;
  ham provider body/header response'a veya loga taşınmaz.

### 4.2 Metrik, Değerlendirme ve Maliyet

- Gecikme, adapter çağrısının uçtan uca süresidir.
- Token değerleri provider cevabından gelir; provider'lar arasında tokenizer
  farkı olabileceği belirtilir.
- JSON validity her başarılı response için çalışır.
- Schema evaluator yalnızca geçerli ve boyut sınırındaki schema verilirse
  çalışır.
- Pricing kaydı bulunan modellerde **tahmini maliyet** gösterilir.
- Fiyatı bulunmayan modelde maliyet `null/unknown` olur; sıfır gösterilmez.
- “Kuruşu kuruşuna kesin maliyet” iddiası kullanılmaz; caching, batch ve özel
  sözleşme indirimlerinin hesaba katılmayabileceği açıklanır.

### 4.3 Recent Result Store

Compare cevabı kriptografik olarak rastgele bir `arena_result_id` içerir.
Backend, kaydetme için gereken orijinal request ve provider sonuçlarını yalnızca
bellekte tutar:

- En fazla 100 compare sonucu.
- En fazla 15 dakika TTL.
- Süreç kapanınca tamamen silinir.
- Eviction en eski kayıttan başlar.

Save endpoint'i tarayıcıdan response, model metriği veya dosya yolu kabul etmez.
Veriyi yalnızca bu store'dan alır. Böylece istemci değiştirilmiş model çıktısını
server sonucuymuş gibi dataset'e yazamaz.

### 4.4 Dataset Writer

`--dataset` yolu sunucu başlamadan önce:

- Absolute ve canonical hale getirilir.
- Mevcut, regular, `.jsonl` uzantılı bir dosya olmak zorundadır.
- `dataset.Reader` ile baştan sona doğrulanır.
- Tek bir `dataset.Writer` olarak açılır ve süreç boyunca paylaşılır.

Tarayıcıdan path alınmaz; yeni dizin veya dosya oluşturulmaz. Writer dosya
descriptor'ı başlangıçta açıldığı için istek sırasında path yeniden çözülmez.
Her append sonrası `Flush` çağrılır. Aynı result ID'nin tekrar kaydedilmesi
engellenir ve üretilen record ID'nin dataset içinde bulunmadığı doğrulanır.

---

## 5. HTTP API Sözleşmesi

### `GET /api/v1/arena/config`

Secret içermeyen başlangıç bilgilerini döndürür:

```json
{
  "models": [
    {"id": "openai/gpt-4.1-mini", "provider": "openai", "available": true},
    {"id": "anthropic/claude-haiku-4-5-20251001", "provider": "anthropic", "available": true}
  ],
  "save_enabled": false,
  "limits": {"max_tokens": 8192, "timeout_ms": 30000}
}
```

`available`, ilgili provider anahtarının süreç başlangıcında mevcut olduğunu
belirtir. Anahtarın değeri, uzunluğu veya bir bölümü asla dönmez.

### `POST /api/v1/arena/compare`

```json
{
  "models": ["openai/gpt-4.1-mini", "anthropic/claude-haiku-4-5-20251001"],
  "messages": [
    {"role": "system", "content": "Return JSON only."},
    {"role": "user", "content": "Categorize this refund request."}
  ],
  "temperature": 0.2,
  "max_tokens": 300,
  "schema": {"type": "object", "required": ["category"]}
}
```

Başarılı kabul edilen istek `arena_result_id` ve iki model sonucu döndürür.
Model sonucunda status, normalize hata, response, latency/token metriği,
opsiyonel tahmini maliyet ve evaluator sonuçları bulunur.

Compare body, Model A ve Model B için iki ID içerir. Server bunları gömülü
allowlist'te doğrular; bilinmeyen model `400`, eksik credential `409` döner.

### `POST /api/v1/arena/save`

Yalnızca save etkinse kullanılabilir:

```json
{
  "arena_result_id": "ar_...",
  "baseline_model": "openai/gpt-4o-mini",
  "use_response_as_expected": true,
  "include_schema": true
}
```

Başarılı cevap:

```json
{
  "status": "saved",
  "record_id": "rec_...",
  "dataset_record_count": 121
}
```

Kaydetme kapalıysa endpoint `404`; result süresi dolmuşsa `410`; bilinmeyen veya
başarısız baseline seçildiyse `400` döndürür.

---

## 6. Güvenlik ve Maliyet Kontrolleri

Arena, loopback olmasına rağmen harici API harcaması ve dosya yazımı yapabildiği
için Phase 10'dan daha sıkı bir istek modeli kullanır:

1. Yalnızca doğrulanmış loopback bind ve `Host` başlığı.
2. Süreç başında `crypto/rand` ile en az 256-bit CSRF tokenı.
3. POST isteklerinde gerçek sunucu origin'iyle birebir `Origin` kontrolü.
4. `X-LLM-Replay-CSRF` header'ı ve sabit-zamanlı token karşılaştırması.
5. Yalnızca `Content-Type: application/json`; CORS başlığı yoktur.
6. `http.MaxBytesReader` ile en fazla 256 KiB request body.
7. `json.Decoder.DisallowUnknownFields` ve tek JSON değerinden sonra EOF.
8. Model seçimi yalnızca gömülü server allowlist'inden yapılır; keyfi model ID
   provider adapter oluşturamaz.
9. En fazla 32 mesaj, mesaj başına 64 KiB ve toplam 128 KiB metin.
10. Temperature `0..2`, max tokens `1..8192`, schema en fazla 64 KiB.
11. Süreç genelinde en fazla iki aktif compare isteği ve dakikada en fazla on
    kabul edilmiş compare isteği.
12. Timeout ve client cancellation bütün provider çağrılarına taşınır.
13. API anahtarı, provider request header'ı ve raw secret hiçbir response veya
    logda görünmez.
14. `Cache-Control: no-store`, mevcut CSP, `nosniff`, frame ve referrer
    politikaları korunur.
15. Dataset append yalnızca CLI ile önceden açılmış writer üzerinden yapılır.

CSRF tokenı server-rendered sayfaya eklenir ve yalnızca aynı origin JavaScript
tarafından header olarak gönderilir. Localhost üzerinde auth eklenmez; ancak
CSRF/Origin kontrolleri ve model allowlist'i zorunludur.

---

## 7. Frontend Kapsamı

- Sistem ve kullanıcı mesajı alanları.
- Server kataloğundan iki model dropdown'ı ve credential availability durumu.
- Temperature, max token ve opsiyonel JSON Schema alanları.
- Provider'a veri gönderileceğini belirten görünür gizlilik/maliyet uyarısı.
- Çalışma sırasında buton kilidi ve model başına loading durumu.
- Yan yana response, normalize hata, latency, input/output token ve tahmini
  maliyet.
- JSON validity ve opsiyonel schema sonucu.
- Phase 10'daki güvenli pretty-print ve sınırlı kelime diff yaklaşımının tekrar
  kullanımı; kullanıcı içeriği `innerHTML` ile eklenmez.
- Hız farkı yalnızca her iki sonuç başarılıysa ve payda sıfır değilse gösterilir.
- Save etkinse baseline seçimi, etkisini açıklayan özet ve dosya yazımından
  hemen önce ikinci onay adımı.
- Responsive, klavye erişilebilir ve sistem temasını izleyen görünüm.

Frontend yeni CDN, font, analytics, framework veya runtime isteği eklemez.
“Sıfır bağımlılık” ifadesi, yeni harici servis/runtime gerekmemesi ve tek binary
dağıtımın korunması anlamında kullanılır; proje zaten Go kütüphaneleri içerir.

---

## 8. Uygulama Sırası

### Phase 11A

1. Arena request/result tipleri ve katı validasyon.
2. Provider adapter factory'sini CLI'dan tekrar kullanılabilir pakete taşıma.
3. Dependency-injected, iki modeli paralel çalıştıran `arena.Runner`.
4. Evaluator/pricing entegrasyonu ve bounded recent-result store.
5. CSRF/Origin/rate-limit korumalı Arena HTTP sunucusu.
6. Read-only karşılaştırma frontend'i.
7. CLI komutu, graceful shutdown ve tarayıcı açma.

### Phase 11B

1. `--dataset` başlangıç doğrulaması ve tek writer lifecycle'ı.
2. Server-side result ID üzerinden baseline seçimi.
3. İdempotent append ve kayıt sayısı güncellemesi.
4. Save UI, özet ve kullanıcı onayı.
5. Eşzamanlı append, symlink ve bozuk dataset testleri.

Phase 11A tüm kabul kriterlerini sağlamadan Phase 11B'ye başlanmaz.

---

## 9. Test Planı

### Runner ve Provider Entegrasyonu

- Mock OpenAI ve Anthropic çağrılarının gerçekten paralel başlaması.
- Tek provider hata/timeout durumunda diğer sonucun korunması.
- Her iki provider hatasının model bazlı sonuç üretmesi.
- Client cancellation ve timeout'un iki çağrıya taşınması.
- Otomatik retry yapılmaması.
- Bilinen/bilinmeyen pricing ve evaluator sonuçları.

### HTTP ve Güvenlik

- Kötü Host, Origin, CSRF tokenı ve content type reddi.
- Büyük body, fazla/uzun mesaj, schema, temperature ve max-token sınırları.
- Web seçiminde bilinmeyen/tekrar eden model reddi ve allowlist dışı model
  enjekte etme denemesinin provider çağrısından önce engellenmesi.
- Dakikalık rate limit ve global concurrency sınırı.
- Bilinmeyen JSON alanı ve trailing JSON reddi.
- Response ve loglarda API anahtarı bulunmaması.
- Ham provider body/header ve `Response.Raw` alanının API/dataset'e sızmaması.
- CORS eklenmemesi ve güvenlik header'ları.

### Dataset Capture

- `--dataset` yokken endpoint'in ve butonun kapalı olması.
- Var olmayan, bozuk, symlink veya `.jsonl` olmayan dataset'in başlangıçta reddi.
- Başarılı sonucu baseline seçerek geçerli `domain.Record` append etme.
- Başarısız modeli baseline seçmenin reddi.
- Client tarafından response/path enjekte edilememesi.
- Aynı result ID'nin tekrar kaydedilmesinde duplicate oluşmaması.
- Eşzamanlı save isteklerinde JSONL satır bütünlüğü.
- Append/flush hatasının başarı gibi raporlanmaması.
- Süresi dolmuş/evict edilmiş result için `410`.

### Frontend ve E2E

- İki model sonucu, metrikler, tahmini maliyet ve partial failure görünümü.
- JSON pretty-print, diff ve 20.000 karakter fallback'i.
- Gizlilik/maliyet uyarısı ve eksik provider anahtarı durumu.
- Save kapalı/açık durumları ve baseline onay akışı.
- Klavye kullanımı, dar ekran ve açık/koyu tema.
- Prompt/model çıktısında XSS regresyonları.

---

## 10. Ölçülebilir Kabul Kriterleri

### Phase 11A

- [x] `llm-replay ui` davranışı ve salt-okunur güvenlik sözleşmesi değişmez.
- [x] `llm-replay arena` modeli web'den seçer ve allowlist dışını reddeder.
- [x] Eksik provider credential UI'da görünür; compare hiçbir provider çağrısı
      başlatmadan `409` ile reddedilir.
- [x] İki mock provider çağrısı paralel başlar ve sıralı çalışmadan daha kısa
      sürede tamamlanır.
- [x] Tek model hata/timeout aldığında diğer response ve metrik görünür kalır.
- [x] Response, latency, token, tahmini maliyet, JSON/schema sonuçları ve güvenli
      diff yan yana gösterilir.
- [x] Bilinmeyen fiyat “ücretsiz” veya `$0` yerine `unknown` gösterilir.
- [x] Origin, CSRF, Host, body/model/parametre ve concurrency sınırı testleri
      geçer.
- [x] Promptların dış sağlayıcılara gönderileceği UI'da açıkça görünür.
- [x] Tek binary dağıtım korunur; frontend harici runtime isteği yapmaz.

### Phase 11B

- [x] Dataset yazımı `--dataset` olmadan hem UI hem backend'de kapalıdır.
- [x] Tarayıcı hiçbir endpoint'e yerel dosya yolu veya provider response'u
      göndermez.
- [x] Yalnızca başarılı ve server store'da bulunan model sonucu baseline olur.
- [x] Kaydedilen satır `dataset.Reader` ve `domain.Record.Validate` kontrolünden
      geçer.
- [x] Save idempotenttir; eşzamanlı çağrılar JSONL'i bozmaz.
- [x] Arbitrary path, symlink değişimi ve bozuk dataset testleri geçer.

---

## 11. Anti-Scope

- `llm-replay ui` içine provider çağrısı veya write endpoint'i eklemek.
- Provider model kataloglarını uzaktan keşfetmek.
- Tarayıcıda API key girmek, saklamak veya göstermek.
- SSE/token streaming ve “ilk dönen sonucu anında basma”.
- İkiden fazla modeli aynı Arena ekranında çalıştırmak.
- Tool/function calling, görsel/audio içerik veya çok-modlu mesajlar.
- Arena oturum geçmişini diske kaydetmek.
- Yeni dataset/dizin oluşturmayı web arayüzüne vermek.
- Genel amaçlı prompt yönetimi veya observability platformuna dönüşmek.
