# 02 — Teknik Mimari ve Sistem Tasarımı

## 1. Mimari Genel Bakış ve Akış Şeması

LLM Replay, yerel dosya sistemi üzerinde çalışan, minimum harici bağımlılığa sahip, yüksek performanslı bir Go CLI aracıdır.

```text
                 ┌────────────────────────┐
                 │  Uygulama (App Backend) │
                 └───────────┬────────────┘
                             │
                             ▼
                 ┌────────────────────────┐
                 │  LLM Replay Capture    │  (127.0.0.1:8787 Reverse Proxy)
                 │  (Secret Redaction)    │
                 └───────────┬────────────┘
                             │
             ┌───────────────┴───────────────┐
             ▼                               ▼
      ┌─────────────┐                 ┌─────────────┐
      │ LLM Provider│                 │   Dataset   │
      │  (Upstream) │                 │  (JSONL)    │
      └─────────────┘                 └──────┬──────┘
                                             │
                                             ▼
                                      ┌─────────────┐
                                      │Replay Engine│  (Worker Pool)
                                      └──────┬──────┘
                                             │
                                ┌────────────┴────────────┐
                                ▼                         ▼
                        ┌──────────────┐          ┌──────────────┐
                        │OpenAI Adapter│          │Anthropic Adp.│
                        └──────┬───────┘          └──────┬───────┘
                               │                         │
                               └────────────┬────────────┘
                                            ▼
                                     ┌──────────────┐
                                     │  Evaluators  │ (JSON, Schema, Match)
                                     └──────┬───────┘
                                            ▼
                                     ┌──────────────┐
                                     │  Comparison  │
                                     │ & Run Report │ (Terminal & JSON)
                                     └──────────────┘
```

---

## 2. Neden Go? (Teknoloji Yığını)

LLM Replay'in çekirdek dili olarak **Go (Golang)** tercih edilmiştir:
1. **Tekil Binary Dağıtımı:** Kullanıcının runtime kurmasına gerek kalmadan `curl` veya `go install` ile anında çalışabilir.
2. **Kusursuz Concurrency (Eşzamanlılık):** Goroutine ve channel yapıları sayesinde yüzlerce isteği yöneten worker pool mimarisi çok hafif ve kararlıdır.
3. **HTTP Reverse Proxy Yeteneği:** Go'nun `net/http/httputil` standart kütüphanesi production kalitesinde proxy yazmak için endüstri standardıdır.
4. **Düşük Kaynak Tüketimi:** Düşük bellek ve CPU ayak izi.
5. **Developer Tooling Ekosistemi:** Kubernetes, Docker, Terraform gibi altyapı araçlarıyla aynı dil kültürünü paylaşır.

### Teknoloji Bileşenleri
* **Dil:** Go 1.22+
* **CLI Çerçevesi:** `github.com/spf13/cobra` (Yalnızca CLI bayrakları ve komut yönlendirmesi için)
* **Veri Depolama:** Line-delimited JSON (`JSONL`)
* **Konfigürasyon:** YAML ve Environment Variables
* **Harici Veritabanı:** MVP'de yok (SQLite ileride opsiyonel run kataloğu için değerlendirilecek)
* **Container:** Dockerfile (Multi-stage build)
* **CI/CD:** GitHub Actions

---

## 3. Repository Organizasyonu

Go topluluğu standartlarına (`golang-standards/project-layout`) uygun, temiz ve içe kapalı (`internal/`) mimari:

```text
llm-replay/
├── cmd/
│   └── llm-replay/
│       └── main.go              # CLI giriş noktası ve Cobra komut entegrasyonu
│
├── internal/
│   ├── capture/                 # Reverse proxy, upstream iletimi, secret sanitization
│   ├── replay/                  # Worker pool, istek normalizasyonu, çalıştırma orkestrasyonu
│   ├── provider/                # Provider arayüzü ve adaptörler
│   │   ├── provider.go          # Normalized request/response interfaces
│   │   ├── openai/              # OpenAI API adaptörü
│   │   └── anthropic/           # Anthropic API adaptörü
│   ├── dataset/                 # JSONL okuyucu/yazıcı, metadata yönetimi, validasyon
│   ├── evaluation/              # Evaluator interface ve kuralları (JSON, Schema, Exact)
│   ├── metrics/                 # Metrik toplayıcı, gecikme yüzdelikleri (P50/P95)
│   ├── pricing/                 # Fiyatlandırma kayıt kütüğü (YAML/JSON) ve hesaplama
│   ├── report/                  # Terminal tablolama ve JSON çıktı formatlayıcıları
│   └── config/                  # Konfigürasyon modelleri ve hiyerarşi yükleyicisi
│
├── examples/
│   ├── datasets/                # Örnek hazır benchmark veri setleri (support.jsonl)
│   └── schemas/                 # Örnek JSON schema dosyaları
│
├── docs/                        # Kapsamlı teknik ve mimari dokümantasyon
├── testdata/                    # Birim ve entegrasyon testleri için mock veriler
├── scripts/                     # Geliştirici ve derleme betikleri
├── .github/
│   └── workflows/               # CI test ve derleme pipeline'ları
├── Dockerfile
├── Makefile
├── go.mod
├── LICENSE
└── README.md
```

> **Önemli Not:** `pkg/` klasörü bilerek oluşturulmamıştır. Harici olarak dış dünyaya SDK gibi açılmayan kodların `internal/` altında tutulması Go best practice'idir.

---

## 4. Çekirdek Domain Tipleri (Core Domain Types)

Sistem, sağlayıcıların (OpenAI, Anthropic vb.) özel JSON şemalarından tamamen bağımsız bir iç domain modeli kullanır:

```go
package domain

import "time"

// Record, dataset dosyasındaki (JSONL) tek bir satırı temsil eder.
type Record struct {
    ID          string                 `json:"id"`
    Timestamp   time.Time              `json:"timestamp"`
    Provider    string                 `json:"provider"`
    Model       string                 `json:"model"`
    Request     Request                `json:"request"`
    Response    Response               `json:"response"`
    Metrics     RecordMetrics          `json:"metrics"`
    Evaluation  *ExpectedEvaluation    `json:"evaluation,omitempty"`
    Status      string                 `json:"status"` // "success", "error", "timeout"
}

// Request, sağlayıcıdan bağımsız normalize edilmiş istek modelidir.
type Request struct {
    Messages    []Message              `json:"messages"`
    Temperature *float64               `json:"temperature,omitempty"`
    TopP        *float64               `json:"top_p,omitempty"`
    MaxTokens   *int                   `json:"max_tokens,omitempty"`
    Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Message, tek bir sohbet mesajını temsil eder.
type Message struct {
    Role    string `json:"role"`    // "system", "user", "assistant"
    Content string `json:"content"` // MVP: Salt metin (Text-only)
}

// Response, sağlayıcıdan bağımsız normalize edilmiş yanıt modelidir.
type Response struct {
    Content      string `json:"content"`
    FinishReason string `json:"finish_reason"`
    Raw          string `json:"raw,omitempty"`
}

// RecordMetrics, tekil isteğin temel çalışma metrikleridir.
type RecordMetrics struct {
    LatencyMs    int64 `json:"latency_ms"`
    InputTokens  int   `json:"input_tokens"`
    OutputTokens int   `json:"output_tokens"`
}

// ExpectedEvaluation, veri setinde opsiyonel olarak bulunabilen doğrulama kriteridir.
type ExpectedEvaluation struct {
    Schema   map[string]interface{} `json:"schema,omitempty"`   // JSON Schema
    Expected string                 `json:"expected,omitempty"` // Exact Match beklenen değer
}
```

---

## 5. Capture Sistemi Tasarımı

Capture için iki yaklaşım incelenmiştir:
* **Mode A — Reverse Proxy:** Uygulama `OPENAI_BASE_URL` adresini `http://localhost:8787/v1` yapar. İstek proxy'den geçer, upstream'e iletilir, yanıt alınır ve o anda diske JSONL olarak yazılır.
* **Mode B — SDK Wrapper:** `ReplayClient(OpenAI())` gibi diller özelinde kütüphaneler yazmak.

### Tercih: Mode A (Reverse Proxy)
1. **Dilden Bağımsız (Polyglot):** Python, Node.js, Go, Java veya Ruby fark etmeksizin tek bir base URL değişikliği ile tüm sistem capture edilebilir.
2. **Sıfır Kod Değişikliği:** Kodda import veya kütüphane değiştirmeye gerek yoktur, sadece ortam değişkeni (env var) yeterlidir.
3. **Provider-Agnostic İlk Adım:** İstek formatı OpenAI API standardında yakalanır ve normalize edilir.

---

## 6. Dataset ve Depolama Mimarisi

* **Format:** Line-delimited JSON (`.jsonl`).
  * **Neden JSONL?** Streaming dostudur, dosya sonuna append etmek basittir, milyonlarca satır olsa dahi belleğe tamamının yüklenmesi gerekmez, `grep`, `head`, `jq` gibi Unix araçlarıyla kolayca analiz edilebilir.
* **Schema Sürümleme:** Olası veri formatı değişikliklerini desteklemek için kayıtlarda `schema_version: "1"` kullanılır.
* **Metadata Dosyası (`.meta.json`):** Dataset klasöründe veri setinin özetini tutar:
  ```json
  {
    "name": "customer-support-production",
    "created_at": "2026-09-10T12:00:00Z",
    "record_count": 250,
    "source": "capture",
    "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
    "tags": ["production", "support"]
  }
  ```

---

## 7. Replay Engine Mimarisi

Replay Engine sistemin kalbidir. Görevleri:
1. Dataset okuma ve doğrulama
2. İstekleri internal `Request` formatına normalize etme
3. Worker Pool üzerinden eşzamanlı çalıştırma
4. Sağlayıcı adaptörlerine dağıtım
5. Hata yönetimi (Timeout & Retry)
6. Metrik ve Evaluation hesaplaması
7. Çalıştırma sonuçlarını diske (`runs/`) yazma

### Replay Run Modeli (Immutable Run)
Her replay işlemi izole ve değiştirilemez bir "Run" nesnesi ve klasörü üretir:
```text
runs/
└── run_20260910_143100/
    ├── config.json       # Çalışma konfigürasyonu ve parametreler
    ├── results.jsonl     # Her bir isteğin aday model yanıtları ve evaluasyonları
    └── summary.json      # Toplu metrikler ve karşılaştırma özeti
```

Run Konfigürasyon Örneği:
```json
{
  "id": "run_20260910_143100",
  "created_at": "2026-09-10T14:31:00Z",
  "dataset_path": "./datasets/support.jsonl",
  "dataset_sha256": "a1b2c3d4...",
  "models": ["openai/gpt-4o-mini", "anthropic/claude-3-5-haiku"],
  "config": {
    "concurrency": 5,
    "timeout_seconds": 30,
    "retries": 3
  }
}
```

---

## 8. Provider Abstraction ve Adaptörler

Replay motoru hiçbir sağlayıcı API'sini doğrudan bilmez. Tüm iletişim `Provider` arayüzü üzerinden yürütülür:

```go
type Provider interface {
    Name() string
    Generate(ctx context.Context, req Request) (*Response, *RecordMetrics, error)
}
```

* **OpenAI Adaptörü:** Normalize edilmiş `Request` nesnesini OpenAI JSON şemasına çevirir, `Authorization: Bearer <KEY>` başlığıyla iletir, yanıtı ve token metriklerini ayrıştırır.
* **Anthropic Adaptörü:** `system` mesajlarını Anthropic'in beklediği ayrı alana alır, `x-api-key` ve `anthropic-version` başlıklarını ekler, `usage` nesnesinden tokenları çeker.

### Kapsam Sınırları:
* **Text-Only:** MVP'de yalnızca metin tabanlı sohbet istekleri desteklenir. Görsel (Vision), ses veya dosya ekleri kapsam dışıdır.
* **Non-Streaming First:** İlk etapta `stream: false` istekler işlenir. Streaming capture passthrough olarak sonraki sürümlere bırakılmıştır.
* **Tool Calling / Function Calling:** İleri sürümlere (V1.1+) bırakılmıştır.

---

## 9. Eşzamanlılık (Concurrency), Timeout ve Yeniden Deneme (Retry)

### Worker Pool
Büyük veri setlerini seri çalıştırmak pratik değildir. Go worker pool yapısı ile kontrollü eşzamanlılık sağlanır:
* CLI Flag: `--concurrency 5` (Varsayılan değer: 5)
* Worker'lar kanaldan (`jobs chan Record`) iş alır ve sonuçları toplama kanalına (`results chan ReplayResult`) iletir.

### Timeout
* CLI Flag: `--timeout 30s` (Varsayılan: 30 saniye).
* Her istek `context.WithTimeout` ile sınırlandırılır. Süre aşımında istek `status = "timeout"` olarak kaydedilir ve işlem kesilmez.

### Retry ve Exponential Backoff
Sadece geçici (transient) hatalar yeniden denenir:
* **Yeniden Denenecek Hatalar:** HTTP 429 (Rate Limit), HTTP 5xx (Sunucu hataları), Geçici ağ kopmaları.
* **Denemeyecek Hatalar:** HTTP 400 (Bad Request), HTTP 401/403 (Auth hatası), Geçersiz parametreler.
* **Politika:** Maksimum 3 deneme. Artan bekleme süresi: `1s`, `2s`, `4s` (+ rastgele jitter).
* HTTP 429 yanıtında `Retry-After` başlığı varsa sistem bu süreye öncelik verir.

---

## 10. Hata Taksonomisi (Error Taxonomy)

Farklı sağlayıcıların hata mesajları ortak bir iç taksonomiye normalize edilir. Bu sayede karşılaştırma raporlarında tutarlılık sağlanır:

```go
type ErrorType string

const (
    ErrAuthentication ErrorType = "authentication_error"
    ErrRateLimit      ErrorType = "rate_limit"
    ErrTimeout        ErrorType = "timeout"
    ErrProviderDown   ErrorType = "provider_error"
    ErrInvalidRequest ErrorType = "invalid_request"
    ErrNetwork        ErrorType = "network_error"
    ErrParse          ErrorType = "parse_error"
    ErrUnknown        ErrorType = "unknown"
)
```

---

## 11. Determinism, Reproducibility ve Veri Doğrulama

LLM çıktıları doğası gereği stokastik (non-deterministic) olabilir. Ancak testin koşulduğu şartların tam tekrarlanabilir olması şarttır:
1. **Dataset SHA256:** Replay başlamadan önce veri setinin hash'i hesaplanıp `run.json` dosyasına yazılır. Böylece hangi verinin test edildiği matematiksel olarak kesinleşir.
2. **Çalışma Parametreleri:** Model, sıcaklık (temperature varsa), seed (destekleniyorsa), concurrency ve pricing sürümü run metadata'sına mühürlenir.

---

## 12. Konfigürasyon Hiyerarşisi

Ayarlar şu öncelik sırasına göre yüklenir:
```text
1. CLI Bayrakları (--concurrency, --timeout, --model)  [En Yüksek Öncelik]
2. Ortam Değişkenleri (OPENAI_API_KEY, LLM_REPLAY_CONCURRENCY)
3. Konfigürasyon Dosyası (llm-replay.yaml)
4. Kod İçi Varsayılan Değerler (Defaults)               [En Düşük Öncelik]
```

---

## 13. Günlükleme (Logging) ve Gözlemlenebilirlik

* Standart terminal kullanıcıları için temiz, formatlı ve renkli çıktılar verilir.
* Hata ayıklama ve loglama için yapılandırılmış JSON loglama (`slog` - Go standard library) kullanılır.
* `--verbose` bayrağı ile detaylı mikro-loglar açılabilir.
* **OpenTelemetry:** Mimari OTEL tracing eklenmesine uygun tasarlanmıştır ancak MVP'de harici collector zorunluluğu getirilmez.

---

## 14. Geliştirme Felsefesi ve Mühendislik İlkeleri

1. **Local-First:** Sıfır bulut bağımlılığı.
2. **CLI-First:** Görsel dashboard öncesi mükemmel çalışan terminal arayüzü.
3. **Provider-Agnostic Core:** Çekirdek mantık hiçbir model sağlayıcısına bağımlı değildir.
4. **File-First Storage:** Veritabanı yerine dosya sistemi (`JSONL`).
5. **Basit ve Anlaşılır Soyutlama:** "İleride lazım olur" düşüncesiyle soyutlama eklenmez.
6. **Mühendislik Sorusunu Asla Unutma:**
   > *"Bu abstraction MVP'deki gerçek bir problemi çözüyor mu, yoksa gelecekte belki kullanırız diye mi ekleniyor?"*
