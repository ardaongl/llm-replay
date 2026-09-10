# 04 — CLI Deneyimi ve UX Tasarımı

## 1. Kullanıcı Deneyimi İlkeleri

LLM Replay'in en büyük farklarından biri geliştirici deneyimidir (Developer Experience - DX).

> **Altın Kural:** Her özellik tasarlanırken şu soru sorulmalıdır:
> *"Geliştirici bunu neden kullanmak istesin?"*

* **Kötü UX Örneği:** 15 adet konfigürasyon dosyası hazırlatmak, arka planda PostgreSQL ayağa kaldırmasını istemek, browser açtırıp hesap oluşturmaya zorlamak.
* **İyi UX (LLM Replay):**
  ```bash
  llm-replay replay dataset.jsonl --model openai/gpt-4o-mini
  ```
  Tek komut, anında çıktı, sıfır bürokrasi.

---

## 2. CLI Komut Mimarisi

LLM Replay, `cobra` kütüphanesi üzerine inşa edilmiş 4 temel komuttan oluşur:

```text
llm-replay
├── capture    # HTTP reverse proxy'yi başlatır ve production isteklerini kaydeder
├── replay     # Veri setini bir veya birden fazla modele karşı yeniden çalıştırır
├── compare    # Farklı replay koşularını (runs) karşılaştırır
└── inspect    # Bir JSONL veri setinin özet istatistiklerini ve içeriğini inceler
```

---

## 3. Komut Kullanım Detayları

### 1. `llm-replay capture`
Localhost üzerinde OpenAI uyumlu bir reverse proxy ayağa kaldırır:
```bash
llm-replay capture \
  --listen 127.0.0.1:8787 \
  --upstream https://api.openai.com \
  --output ./datasets/production.jsonl
```
* **Kullanım:** Uygulamanızın ortam değişkenini `OPENAI_BASE_URL=http://localhost:8787/v1` olarak ayarlamanız yeterlidir.
* Gelen tüm istekler upstream'e iletilir, yanıt alınır ve hassas başlıklar (API Key vb.) temizlenerek belirtilen `.jsonl` dosyasına eklenir (append-only).

---

### 2. `llm-replay replay`
Bir veri setini hedef aday model(ler)e gönderir:
```bash
# Tek model ile çalıştırma:
llm-replay replay ./datasets/production.jsonl \
  --model openai/gpt-4o-mini \
  --concurrency 5 \
  --timeout 30s

# Çoklu model karşılaştırmalı çalıştırma:
llm-replay replay ./datasets/production.jsonl \
  --model openai/gpt-4o-mini \
  --model anthropic/claude-3-5-haiku \
  --eval json \
  --output ./runs/
```
* **Opsiyonel Bayraklar:**
  * `--concurrency <sayı>`: Paralel worker sayısı (Varsayılan: 5).
  * `--timeout <süre>`: İstek başı zaman aşımı süresi (Varsayılan: 30s).
  * `--pricing <dosya>`: Özel birim fiyatlandırma kütüğü (`pricing.yaml`).
  * `--eval <tür>`: Aktif edilecek değerlendiriciler (`json`, `schema`, `match`).

---

### 3. `llm-replay compare`
Daha önce çalıştırılmış replay sonuçlarını karşılaştırır:
```bash
# Tek bir multi-model run'ı karşılaştırma:
llm-replay compare runs/run_20260910_143100

# İki ayrı bağımsız run'ı karşılaştırma:
llm-replay compare runs/run_01 runs/run_02
```

---

### 4. `llm-replay inspect`
Bir veri setini çalıştırmadan önce sağlık ve dağılım kontrolü yapar:
```bash
llm-replay inspect datasets/production.jsonl
```

Örnek Terminal Çıktısı:
```text
Dataset Overview: datasets/production.jsonl
────────────────────────────────────────────────────────
Total Requests       : 428
Successful Captures  : 421
Failed Captures      : 7

Token Distributions
  Input Tokens  (P50): 420    (P95): 2,310    (Max): 8,721
  Output Tokens (P50): 110    (P95): 450      (Max): 1,200

Structured Data
  Valid JSON Outputs : 398 (94.5%)
  Contains Evaluation: 120 (28.5%)
```

---

## 4. Terminal Kullanıcı Arayüzü (Pretty UX)

Replay çalışırken kullanıcıya anlık ilerleme durumu ve bitiminde temiz bir karşılaştırma tablosu sunulur:

```text
LLM Replay — Model Migration Benchmark

Dataset      : customer-support.jsonl (250 requests)
Candidates   : [1] openai/gpt-4o-mini
               [2] anthropic/claude-3-5-haiku
Concurrency  : 5 workers | Timeout: 30s

Running Replay...
[████████████████████████████████████████] 250/250 (100%)

RESULTS COMPARISON
──────────────────────────────────────────────────────────────────
Metric                  Model A (GPT-4o-mini)  Model B (Claude-3.5-Haiku)
──────────────────────────────────────────────────────────────────
Success Rate            99.2%                  99.6%
JSON Validity           96.8%                  99.2%
Schema Adherence        92.0%                  97.6%
Exact Match             91.2%                  94.0%

Latency (Avg)           1.12s                  1.05s
Latency (P50)           842ms                  911ms
Latency (P95)           1.94s                  1.71s

Input Tokens            182,400                182,400
Output Tokens           42,100                 38,900
Total Tokens            224,500                221,300

Estimated Cost          $0.0526                $0.0942
──────────────────────────────────────────────────────────────────
Output Artifacts Saved: ./runs/run_20260910_143100/
```

> **Not:** Sistem MVP aşamasında otomatik bir "Kazanan (Winner)" seçmez. Çünkü hız, maliyet veya schema doğruluğu arasındaki tradeoff kararı geliştiriciye aittir.

---

## 5. Rapor ve Çıktı Dosya Hiyerarşisi

Her replay koşusu izole bir alt dizine kaydedilir:
```text
runs/
└── run_20260910_143100/
    ├── config.json      # Kullanılan konfigürasyon, model parametreleri ve dataset hash'i
    ├── results.jsonl    # İstek istek tüm aday yanıtları, gecikmeleri ve evaluation sonuçları
    └── summary.json     # CI/CD araçları tarafından kolayca parse edilebilen metrik özeti
```

### Örnek `summary.json`:
```json
{
  "run_id": "run_20260910_143100",
  "created_at": "2026-09-10T14:31:00Z",
  "dataset": "customer-support.jsonl",
  "dataset_sha256": "8a35b6f...",
  "total_requests": 250,
  "models": {
    "openai/gpt-4o-mini": {
      "success_rate": 0.992,
      "latency_avg_ms": 1120,
      "latency_p50_ms": 842,
      "latency_p95_ms": 1940,
      "input_tokens": 182400,
      "output_tokens": 42100,
      "estimated_cost_usd": 0.0526,
      "evaluations": {
        "json_valid_rate": 0.968,
        "schema_adherence_rate": 0.920,
        "exact_match_rate": 0.912
      }
    },
    "anthropic/claude-3-5-haiku": {
      "success_rate": 0.996,
      "latency_avg_ms": 1050,
      "latency_p50_ms": 911,
      "latency_p95_ms": 1710,
      "input_tokens": 182400,
      "output_tokens": 38900,
      "estimated_cost_usd": 0.0942,
      "evaluations": {
        "json_valid_rate": 0.992,
        "schema_adherence_rate": 0.976,
        "exact_match_rate": 0.940
      }
    }
  }
}
```

---

## 6. Örnek Demo Senaryosu (Customer Support Classifier)

Repository'nin vitrini (demo senaryosu) için kurgulanan kullanım:

* **Senaryo:** Müşteri destek taleplerini sınıflandıran ve aciliyet belirleyen backend servisi.
* **Girdi:**
  ```text
  "I was charged twice for my annual subscription. Please refund the extra charge immediately."
  ```
* **Beklenen JSON Çıktı:**
  ```json
  {
    "category": "billing",
    "urgency": "high"
  }
  ```
* 100-250 adet gerçekçi production isteği `examples/datasets/support.jsonl` olarak depolanır.
* Kullanıcı tek komutla bu hazır veri setini farklı modeller üzerinde test ederek model geçişinin fizibilitesini saniyeler içinde kanıtlar.
