# 01 — Vizyon, Problem ve Kapsam Dokümanı

## 1. Dokümanın Amacı

Bu doküman, **LLM Replay** projesinin neden geliştirildiğini, hangi problemi çözdüğünü, kimler için tasarlandığını, ürün vizyonunu, MVP kapsamını, GitHub/Open Source ve kariyer (portfolio) konumlandırmasını eksiksiz bir şekilde tanımlar.

Projeyi sıfırdan geliştirecek veya inceleyecek herhangi bir mühendis ya da yapay zeka ajanı, önceki hiçbir konuşmaya gerek kalmaksızın tüm işlevsel ve felsefi bağlamı burada bulabilmelidir.

> **Ana İlke:** Küçük scope'lu, gerçek bir production problemini çözen, teknik olarak temiz, güçlü bir README ve demo ile açık kaynakta profesyonel görünen bir AI infrastructure projesi geliştirmek. Gereksiz büyük bir enterprise sistem inşa edilmemelidir.

---

## 2. Proje Adı ve Konumlandırma

* **Çalışma Adı:** `LLM Replay` (Repository ve teknik mimaride `llm-replay`)
* **Kısa Açıklama:**
  > Capture, replay, benchmark and compare production LLM requests across models.
* **Açıklayıcı Tanım:**
  > An open-source toolkit for capturing LLM traffic and replaying real production requests against different models, prompts and configurations to measure quality, latency, reliability and cost.
* **GitHub Tagline:**
  > Replay real LLM workloads. Compare models, prompts, cost, latency and structured-output reliability before shipping changes.
* **GitHub Topics:**
  `llm`, `ai`, `evaluation`, `llm-evaluation`, `developer-tools`, `observability`, `openai`, `anthropic`, `golang`, `ai-infrastructure`

---

## 3. Temel Problem

Production ortamında LLM kullanan ekiplerin karşılaştığı en kritik zorluk şudur:

Bir uygulama halihazırda belirli bir model/prompt/parametre kullanmaktadır:
* **Model Değişimi:** `GPT-4o` → `GPT-4o-mini` veya `OpenAI` → `Claude 3.5 Sonnet`
* **Prompt İterasyonu:** `Prompt v1` → `Prompt v2`
* **Hiperparametre:** `temperature: 0.7` → `temperature: 0.2`
* **Maliyet Optimizasyonu:** Pahalı model → Ucuz model

**Cevapsız Kalan Kritik Soru:**
> *"Bu değişiklik production'daki gerçek kullanıcı isteklerinde nasıl davranacak?"*

### Mevcut Yanlış Yaklaşım
Mühendisler genellikle 3-5 adet manuel prompt dener:
* Prompt 1 → İyi
* Prompt 2 → İyi
* Prompt 3 → İyi
...ve ardından değişikliği doğrudan production'a dağıtır.

**Bu yöntem tehlikelidir ve güvenilmezdir!** Çünkü gerçek production iş yükü (workload) şunları barındırır:
* Beklenmeyen uç durumlar (edge-case promptlar),
* Çok uzun bağlamlar (long contexts),
* Farklı diller ve yerelleştirme anomalileri,
* Farklı JSON ve structured output gereksinimleri,
* Değişken token uzunlukları,
* Tool-call ve function-calling senaryoları,
* Kullanıcı kaynaklı beklenmeyen/hatalı girdiler.

### Gerçek Çözüm İhtiyacı
> Production trafiğindeki gerçek veya sanitize edilmiş LLM isteklerini bir **dataset** haline getirmek; ardından bu birebir aynı iş yükünü farklı model ve konfigürasyon kombinasyonlarına **replay (yeniden oynatma)** ederek güvenle karşılaştırabilmektir.

---

## 4. Ana Kullanım Senaryosu ve Zihinsel Model

Normal bir şirket mimarisinde istek akışı:
```text
User ──→ Application Backend ──→ OpenAI ──→ Response
```

LLM Replay devreye girdiğinde:
```text
User ──→ Application Backend ──→ LLM Replay Capture Proxy ──→ OpenAI
```

Capture katmanı istek akarken arka planda şeffaf bir şekilde metadata toplar:
* Request payload (messages, temperature, model)
* Response payload
* Latency (gecikme süresi - ms)
* Token kullanımı (input_tokens, output_tokens)
* Estimated cost (tahmini maliyet)
* Timestamp & Status (success / error)
* Structured output doğruluğu

Toplanan bu istekler yerel bir `JSONL` datasetine dönüşür. Daha sonra geliştirici tek bir komutla bu yükü yeniden oynatır:
```bash
llm-replay replay production.jsonl \
  --model openai/gpt-4o-mini \
  --model anthropic/claude-3-5-sonnet
```

Ve karşılaştırma raporunu anında terminalde görür:
```text
Dataset: production-2026-09-10
Requests: 250

MODEL COMPARISON

                         Model A       Model B
Success rate             98.4%         99.2%
JSON validity            94.1%         99.0%
Schema adherence         91.8%         97.7%

Avg latency              1.42s         1.81s
P95 latency              2.90s         3.31s

Input tokens             1.2M          1.2M
Output tokens            240K          211K

Estimated cost           $4.82         $3.41
```

### Zihinsel Model: "LLM Sistemleri İçin Git ve Test Mantığı"
Geleneksel yazılım dünyasındaki test döngüsü:
```text
Kod Değişikliği ──→ Testleri Çalıştır (Unit/Integration) ──→ Deploy
```

LLM tabanlı modern sistemlerde olması gereken döngü:
```text
Model / Prompt Değişikliği ──→ Production Dataset'ini Replay Et ──→ Evaluate ──→ Deploy
```

---

## 5. Hedef Kullanıcılar

1. **AI Engineers:** Model veya sistem promptu değişikliklerini gerçek veriyle test etmek isteyenler.
2. **Backend Engineers:** LLM API'leri kullanan production backend servislerini yöneten ve SLA/maliyet takibi yapanlar.
3. **ML Platform / AI Platform Engineers:** Farklı model ve sağlayıcılar arasında sistematik benchmark yapan ekipler.
4. **Forward Deployed Engineers (FDE):** Müşteri sistemlerinde daha ucuz/hızlı model, farklı provider geçişi ve structured output güvenilirliği ölçen mühendisler.
5. **Startuplar:** OpenAI, Anthropic, Gemini gibi sağlayıcılar arasında fatura şişkinliği veya performans nedeniyle güvenli geçiş planlayan ekipler.

---

## 6. Projenin Ana Değeri ve Sınırları (Anti-Scope)

### Ne Olduğu:
* Real workload'lar üzerinde model/prompt/config değişikliklerini güvenli biçimde replay ve compare eden bir **test ve regresyon aracıdır**.

### Ne Olmadığı:
* Bir chatbot değildir.
* Bir AI uygulama framework'ü (LangChain/LlamaIndex) değildir.
* Bir observability dashboard'u değildir.
* Bir **LangSmith** veya **Langfuse** klonu değildir.

### MVP'nin Bilerek Çözmeyeceği Şeyler (Anti-Scope):
Kapsamın kontrolsüz büyümesini engellemek için ilk sürüme kesinlikle alınmayacaklar:
* Dağıtık mimari (Distributed architecture)
* Kubernetes & Kafka
* Web dashboard ve görsel UI
* Authentication, User Accounts, Organization / Team yönetimi, RBAC
* Cloud-hosted SaaS ve Billing altyapısı
* Realtime canlı dashboardlar
* Karmaşık tracing ve agent orchestration
* Vektör veritabanları
* Prompt yönetim platformu ve dataset etiketleme arayüzü
* Otomatik prompt optimize edici
* Tam teşekküllü LLM observability platformu

---

## 7. Temel Ürün İş Akışı (Product Workflow)

Sistemin uçtan uca veri hattı:
```text
Production LLM Requests
        │
        ▼
   Capture Layer (Reverse Proxy)
        │
        ▼
   Replay Dataset (JSONL)
        │
        ▼
   Replay Engine (Worker Pool)
        │
        ▼
   Provider Adapters (OpenAI, Anthropic)
        │
        ▼
   Candidate Responses
        │
        ▼
   Evaluators (JSON, Schema, Exact Match)
        │
        ▼
   Metrics Aggregator (Latency, Cost, Tokens, Reliability)
        │
        ▼
   CLI Terminal Report & JSON Summary
```

---

## 8. MVP İçin Nihai Kapsam Özeti

* **Dil & Dağıtım:** Go ile tek binary (Single executable CLI)
* **Veri Formatı:** Local JSONL veri setleri (Local-first, sıfır harici DB)
* **Desteklenen İstek Tipi:** OpenAI-compatible text-only, non-streaming istekler
* **Provider Desteği:** 
  1. OpenAI Adaptörü (Öncelikli)
  2. Anthropic Adaptörü (Cross-provider benchmark için)
* **Replay Motoru:** Concurrency kontrollü worker pool, retry (exponential backoff) ve context timeout
* **Metrikler:** Success rate, error rate, P50/P95 latency, token kullanımı, tahmini maliyet (pricing registry)
* **Evaluators:** JSON validity, JSON Schema adherence, Exact match
* **Raporlama:** Zengin terminal çıktıları (Pretty tables) ve CI dostu JSON rapor dosyaları (`summary.json`)
* **Capture:** Local OpenAI-compatible HTTP reverse proxy (`127.0.0.1`)
* **Güvenlik:** Secret redaction (API Key ve Bearer token'ların kaydedilmemesi)
* **Dokümantasyon & Vitrin:** Yüksek kaliteli README, örnek dataset ve demo senaryosu

---

## 9. Mühendislik ve Portfolio Konumlandırması

Bu proje, geliştirenin GitHub profilinde ve mülakatlarda şu yetkinlikleri somutlaştırmalıdır:
```text
AI Engineering ── Backend Engineering ── LLM Infrastructure ── Evaluation ── Observability
API Design ── CLI Design ── Concurrency ── Data Modeling ── Production Thinking
Cost Optimization ── Developer Tooling
```

### CV & LinkedIn Anlatımı:
> *"Built an open-source LLM regression testing and replay toolkit in Go that captures real AI workloads and benchmarks model migrations across reliability, structured-output correctness, latency and cost."*
> 
> *Kısa Versiyon:*
> *"Built a local-first LLM replay and evaluation engine for testing model migrations against production workloads."*

### Forward Deployed Engineer (FDE) Bağlantısı:
Müşteri *"OpenAI faturalarımız çok yüksek, tasarruf etmek istiyoruz"* dediğinde; FDE production trafiğini yakalar, daha ucuz modele replay eder ve şu net veriyi masaya koyar:
> *"Aday Model: %38 daha ucuz, %12 daha hızlı, schema doğruluğu %0.7 daha yüksek, anlamsal kalite farkı yalnızca %1.4. Geçiş onaylanabilir."*

### AI Backend Engineer Bağlantısı:
Proje hem backend sistem derinliğini (HTTP proxy, goroutine worker pools, context timeouts, exponential backoff, rate limiting, data serialization) hem de AI domain derinliğini (token economics, model drift, prompt workloads, non-deterministic evaluation) harmanlar.

---

## 10. MVP Definition of Done (DoD)

MVP tamamlanmış kabul edilmek için bir mühendis şu adımları hatasız çalıştırabilmelidir:

1. **Dataset İnceleme:**
   ```bash
   llm-replay inspect examples/support.jsonl
   ```
2. **Tek Model ile Replay:**
   ```bash
   llm-replay replay examples/support.jsonl --model openai/gpt-4o-mini
   ```
3. **Çoklu Model (Cross-Provider) Karşılaştırma:**
   ```bash
   llm-replay replay examples/support.jsonl \
     --model openai/gpt-4o-mini \
     --model anthropic/claude-3-5-haiku
   ```
4. **Metrikleri ve Evaluator Çıktılarını Görme:**
   Başarı oranı, P50/P95 gecikme, token tüketimi, maliyet, JSON geçerliliği ve schema uyumunun hem terminalde hem `summary.json`'da üretilmesi.
5. **Proxy Üzerinden Canlı İstek Yakalama:**
   Geliştirici uygulamasının base URL'ini `http://localhost:8787/v1` yaparak isteklerin dataset'e secret'lar temizlenerek yazılması.
6. **Local-First Güvencesi:** Tüm verinin yerel dosya sisteminde kalması.
7. **5 Dakikada Demo:** Repo klonlandıktan sonra 5 dakika içinde binary derlenip örnek benchmark çalıştırılabilmesi.

---

## 11. Nihai Vizyon Özeti

> **"Before changing a production LLM, replay reality."**
