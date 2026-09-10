# LLM Replay — Dokümantasyon Merkezi (Master Index)

Bu dizin, **LLM Replay** projesinin tüm vizyonunu, teknik mimarisini, veri modellerini, evaluation/metrik sistemini, CLI tasarımını, güvenlik prensiplerini, gelecek yol haritasını ve adım adım geliştirme fazlarını (Phase 0'dan Phase 9'a) eksiksiz olarak içermektedir.

Hiçbir bilgi azaltılmamış veya atlanmamıştır. Projeyi sıfırdan geliştirecek bir mühendis veya AI agent, geçmiş konuşmalara ihtiyaç duymadan bu dokümanlar üzerinden projeyi uçtan uca anlayabilir ve inşa edebilir.

---

## 📚 Dokümantasyon Haritası

### 1. Temel Dokümanlar
* **[01. Vizyon, Problem ve Kapsam Dokümanı](file:///c:/Users/ardao/Desktop/llm-replay/docs/01-vision-and-scope.md)**
  * Dokümanın Amacı, İsim ve Tagline'lar
  * Temel Problem (Production LLM Değişiklikleri Riski)
  * Ana Kullanım Senaryosu & Zihinsel Model ("LLM Systems için Git/Test Mantığı")
  * Hedef Kitle (AI Engineers, Backend, ML Platform, FDE, Startuplar)
  * Portfolio, CV/LinkedIn ve Mühendislik Konumlandırması (FDE & AI Backend)
  * Ürün İş Akışı (Capture → Dataset → Replay → Evaluate → Compare → Report)
  * MVP Kapsamı ve Bilerek Dışarıda Bırakılanlar (Anti-Scope)
  * Definition of Done (DoD) ve İlk Release Başarı Kriteri

* **[02. Teknik Mimari ve Sistem Tasarımı](file:///c:/Users/ardao/Desktop/llm-replay/docs/02-architecture-and-design.md)**
  * Mimari Diyagramı ve Bileşenler
  * Go Dili Tercih Gerekçeleri ve Teknoloji Yığını
  * Repository Organizasyonu ve Paket Yapısı (`internal/` vs `pkg/`)
  * Çekirdek Domain Tipleri (`Record`, `Request`, `Response`, `ReplayRun` vb.)
  * Capture Tasarımı (Proxy Mode A vs SDK Mode B Karşılaştırması)
  * Dataset & JSONL Formatı, Schema Sürümleme, Metadata Yapısı
  * Replay Engine Tasarımı (Worker Pool, Concurrency, Timeout, Retry)
  * Provider Mimarisi ve Normalizasyon (OpenAI, Anthropic)
  * Text-Only ve Non-Streaming Kapsam Kararları
  * Hata Taksonomisi (Error Taxonomy) ve Normalizasyon
  * Determinism, Reproducibility ve Dataset Hashing (SHA256)
  * Konfigürasyon Hiyerarşisi (Flags → Env → File → Defaults)
  * Logging, Observability ve OpenTelemetry Yaklaşımı
  * Geliştirme Felsefesi ve Mühendislik Kalite Beklentileri

* **[03. Evaluation ve Metrikler Sistemi](file:///c:/Users/ardao/Desktop/llm-replay/docs/03-evaluations-and-metrics.md)**
  * Metrics vs. Evaluators Kavramsal Ayrımı
  * Metrik Kategorileri (Reliability, Performance P50/P95, Usage, Cost)
  * Provider Fiyatlandırma Kaydı (Pricing Registry, YAML/JSON, Sürümleme)
  * Evaluator Interface ve Modüler Tasarım
  * JSON Validity Evaluator
  * JSON Schema Adherence Evaluator
  * Exact Match Evaluator
  * Baseline (Production Response) vs. Candidate Karşılaştırma Mantığı
  * İleri Aşama: LLM-as-a-Judge Tasarımı ve Neden MVP Sonrasına Kaldığı

* **[04. CLI Deneyimi ve UX Tasarımı](file:///c:/Users/ardao/Desktop/llm-replay/docs/04-cli-and-ux.md)**
  * CLI Felsefesi: "Tek Komutla Sonuç Alma"
  * Temel Komutlar: `capture`, `replay`, `compare`, `inspect`
  * Terminal Arayüzü (Pretty Tables, Progress Bars, Summary)
  * Replay Run Model & Dosya Çıktı Hiyerarşisi (`runs/run_id/...`)
  * Örnek Uçtan Uca Demo Senaryosu (Customer Support Classifier)

* **[05. Güvenlik, Gizlilik ve Ağ Politikası](file:///c:/Users/ardao/Desktop/llm-replay/docs/05-security-and-privacy.md)**
  * Temel İlke: Local-First Gizlilik (Sıfır Bulut Bağımlılığı)
  * Secret Filtering (API Key, Bearer Token, Cookie Redaction)
  * Proxy Ağ Güvenliği (127.0.0.1 Localhost-Only Bind)
  * Veri Sanitizasyonu ve Gelecek PII Maskeleme Planı

* **[06. Gelecek Yol Haritası ve İleri Özellikler](file:///c:/Users/ardao/Desktop/llm-replay/docs/06-future-roadmap.md)**
  * MVP Sonrası Öncelik Sıralaması
  * CI/CD Entegrasyonu ve Regresyon Eşikleri (`llm-replay check`)
  * Diff Görünümü (CLI/HTML)
  * Dataset Sampling ve Filtering
  * Prompt Varyantları (Prompt V1 vs V2 Replay)
  * Provider Migration Araç Kiti
  * Tool Calling / Function Calling Desteği
  * Streaming Capture / Passthrough
  * SQLite Run/Dataset Kataloğu
  * HTML Görsel Dashboard

---

### 2. Ayrıntılı Faz Geliştirme Dokümanları (`docs/phases/`)

Geliştirme sürecinin aşama aşama, kontrollü ve test odaklı ilerlemesi için hazırlanmış spesifik uygulama kılavuzları:

| Faz | Doküman | Odak Noktası |
| :--- | :--- | :--- |
| **Phase 0** | **[Phase 0 — Repository Foundation](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-00-foundation.md)** | İskelet kurulum, Go mod, Cobra temel, Makefile, Dockerfile, GH Actions, temel config. |
| **Phase 1** | **[Phase 1 — Domain & Dataset Engine](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-01-domain-and-dataset.md)** | Core domain structs, JSONL reader/writer, dataset metadata, roundtrip testleri. |
| **Phase 2** | **[Phase 2 — Provider Interface & OpenAI Adapter](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-02-provider-openai.md)** | Provider interface, OpenAI API adaptörü, token parsing, hata taksonomisi, mock testler. |
| **Phase 3** | **[Phase 3 — Replay Engine & Worker Pool](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-03-replay-engine.md)** | Worker pool eşzamanlılığı, timeout, backoff retry, run metadata kaydı. |
| **Phase 4** | **[Phase 4 — Metrics & Pricing Registry](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-04-metrics-and-pricing.md)** | Reliability, latency P50/P95, token hesabı, pricing YAML registry ve maliyet motoru. |
| **Phase 5** | **[Phase 5 — Modular Evaluators](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-05-evaluators.md)** | Evaluator interface, JSON validity, JSON Schema adherence, Exact match. |
| **Phase 6** | **[Phase 6 — CLI Interface & Terminal UX](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-06-cli-interface.md)** | Cobra komutları (`replay`, `inspect`, `compare`), terminal tabloları, JSON çıktıları. |
| **Phase 7** | **[Phase 7 — Capture Proxy & Sanitization](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-07-capture-proxy.md)** | Reverse HTTP proxy, OpenAI-uyumlu capture, secret redaction, JSONL append. |
| **Phase 8** | **[Phase 8 — Anthropic Provider Adapter](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-08-provider-anthropic.md)** | Anthropic Messages API adaptörü, format çevrimi, cross-provider karşılaştırma testi. |
| **Phase 9** | **[Phase 9 — Polish, Demo & Release](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-09-polish-and-release.md)** | README vitrini, Customer Support fixture dataset, benchmark demosu, CI release pipeline. |
| **Phase 10** | **[Phase 10 — Secure Web Reports & Run Comparison](file:///c:/Users/ardao/Desktop/llm-replay/docs/phases/phase-10-web-report-ui.md)** | Çoklu-run artifact katmanı, önce bağımsız HTML raporu, ardından loopback-only yerel UI, güvenli diff ve ölçek sınırları. |

---

## 🎯 Projenin Temel İlkeleri (Golden Rules)

1. **LLM Replay bir production LLM regression/replay aracıdır;** LangSmith/Langfuse klonu veya genel bir observability platformu değildir.
2. **Local-First & CLI-First:** Bulut bağımlılığı ve veritabanı kurulum zorunluluğu yoktur. Veriler yerel dosya sistemindedir (`JSONL`).
3. **Mühendislik Sadeligi:** Yalnızca MVP'deki gerçek problemleri çözen yapılar kurulur; "ileride lazım olur" gerekçesiyle gereksiz soyutlama ve overengineering yapılmaz.
4. **Gizlilik:** Production secret'ları (API key, bearer token) asla diske kaydedilmez.
