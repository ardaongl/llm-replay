# 03 — Evaluation ve Metrikler Sistemi

## 1. Metrics vs. Evaluators Kavramsal Ayrımı

LLM Replay mimarisinde temiz bir sistem tasarımı için **Metrikler** ve **Değerlendiriciler (Evaluators)** kesin çizgilerle birbirinden ayrılmıştır:

```text
SİSTEM ÇIKTILARI
├── Metrics (Objektif / Sistem Kaynaklı Ölçümler)
│   ├── Reliability (Success, Error, Timeout Oranları)
│   ├── Performance (Gecikme, P50, P95)
│   ├── Token Usage (Input, Output, Toplam Token)
│   └── Cost (Tahmini İstek ve Toplam Maliyet)
│
└── Evaluators (İçerik / Semantik Doğrulama Kuralları)
    ├── JSON Validity (Çıktının geçerli JSON olup olmadığı)
    ├── Schema Adherence (Belirlenen JSON Schema'ya tam uyum)
    ├── Exact Match (Beklenen çıktı ile birebir eşleşme)
    └── LLM-as-a-Judge (İleri aşama: Model destekli anlamsal puanlama)
```

> **Mimari İlke:** Gecikme süresi veya token sayısı bir "evaluator" değil, doğrudan request-response döngüsünden okunan bir sistem metriğidir. Evaluator'lar ise yanıt içeriğini (`content`) belirli bir kurala göre test eden fonksiyonel filtrelerdir.

---

## 2. Metrikler Sistemi (Metrics)

İlk sürümde toplanacak ve raporlanacak çekirdek metrikler:

### A. Reliability (Güvenilirlik)
* **Success Rate (%):** Başarılı tamamlanan isteklerin toplam isteğe oranı.
* **Error Rate (%):** Sağlayıcı veya ağ hatası alan isteklerin oranı.
* **Timeout Rate (%):** Belirlenen süre zarfında yanıt veremeyen isteklerin oranı.

### B. Performance (Gecikme Süreleri)
* **Ortalama Gecikme (Average Latency - ms):** Tüm başarılı isteklerin ortalama süresi.
* **P50 Latency (Medyan Gecikme - ms):** İsteklerin %50'sinin tamamlandığı tavan süre.
* **P95 Latency (95. Yüzdelik Gecikme - ms):** İsteklerin %95'inin tamamlandığı süre (SLA ve kuyruk gecikmeleri için kritik).
* *(P99 metrikleri ileriki sürümlere bırakılmıştır).*

### C. Usage (Token Tüketimi)
* **Input Tokens:** İstek bazında ve toplam girdi token sayısı.
* **Output Tokens:** Model tarafından üretilen ve toplam çıktı token sayısı.
* **Total Tokens:** Input + Output token toplamı.

### D. Cost (Tahmini Maliyet)
* **Tahmini İstek Başı Maliyet ($):** İlgili modelin birim fiyat tarifesi üzerinden hesaplanan istek maliyeti.
* **Toplam Run Maliyeti ($):** Replay edilen tüm veri setinin aday modeldeki toplam faturası.
* **Ortalama İstek Maliyeti ($):** Toplam maliyet / Başarılı istek sayısı.

---

## 3. Fiyatlandırma Sistemi (Pricing Registry)

Maliyet hesaplaması kod içerisine sabit (hard-coded) olarak gömülmez. Bunun yerine sürüm kontrolü yapılabilen, harici ve üzerine yazılabilir (overridable) bir fiyatlandırma sicili (`pricing registry`) kullanılır.

### Örnek Fiyatlandırma Kaydı (`pricing.yaml`):
```yaml
version: "2026-09-01"
models:
  openai/gpt-4o:
    input_per_million: 5.00
    output_per_million: 15.00
  openai/gpt-4o-mini:
    input_per_million: 0.15
    output_per_million: 0.60
  anthropic/claude-3-5-sonnet:
    input_per_million: 3.00
    output_per_million: 15.00
  anthropic/claude-3-5-haiku:
    input_per_million: 0.25
    output_per_million: 1.25
```

### Kullanıcı Tarafından Özelleştirme:
Geliştiriciler kendi kurumsal indirimlerini veya güncel fiyatları CLI üzerinden geçirebilir:
```bash
llm-replay replay dataset.jsonl \
  --model openai/gpt-4o-mini \
  --pricing my-enterprise-pricing.yaml
```

---

## 4. Evaluator Mimarisi ve Arayüzü

Evaluator'lar modüler bir `Evaluator` interface'i ile tanımlanır:

```go
type EvaluationResult struct {
    Name    string                 `json:"name"`
    Passed  bool                   `json:"passed"`
    Score   float64                `json:"score,omitempty"` // 0.0 - 1.0 arası opsiyonel puan
    Details map[string]interface{} `json:"details,omitempty"`
    Error   string                 `json:"error,omitempty"`
}

type Evaluator interface {
    Name() string
    Evaluate(ctx context.Context, record *Record, candidateResponse *Response) EvaluationResult
}
```

---

## 5. Çekirdek Evaluator'lar

### 1. JSON Validity Evaluator (`json_valid`)
Model çıktısının geçerli bir JSON formatında olup olmadığını kontrol eder:
* Yanıt boşluklardan arındırılır.
* `json.Valid([]byte(response.Content))` ile doğrulanır.
* Geçerliyse: `Passed = true`
* Hatalıysa veya kırpılmışsa: `Passed = false`, hata mesajı `Details` içerisine yazılır.
* Özellikle structured output üreten modellerin sağlamlığını test etmek için en kritik temel filtredir.

### 2. JSON Schema Adherence Evaluator (`schema_adherence`)
Eğer dataset kaydında beklenen bir şema tanımlanmışsa (`record.Evaluation.Schema`), model yanıtının bu JSON Şemasına tam uyup uymadığını kontrol eder:
* Yanıt JSON olarak parse edilir.
* Şemadaki zorunlu alanlar (`required`), tip tanımları (`type: string, number, array vb.`) kontrol edilir.
* Model migration süreçlerinde aday modelin beklenen veri yapısını bozup bozmadığı ölçülür.

### 3. Exact Match Evaluator (`exact_match`)
Sınıflandırma (classification), kategori belirleme veya duygu analizi gibi deterministik senaryolarda kullanılır:
* Dataset içerisindeki `record.Evaluation.Expected` alanı ile adayın `response.Content` alanı karşılaştırılır (büyük/küçük harf ve boşluk normalizasyonu uygulanabilir).
* Eşleşiyorsa `Passed = true`, aksi takdirde `Passed = false`.

---

## 6. Baseline Kavramı (Production vs. Candidate)

Replay motorunun en ayırt edici gücü **Baseline (Mevcut Durum)** ile **Candidate (Aday Durum)** karşılaştırmasıdır:

```text
Production'da Gerçekleşmiş Yanıt (JSONL Kaydı)
                      │
                      ▼
               Baseline Response
                      ▲
                      │  (Karşılaştırma & Diff)
                      ▼
              Candidate Response
                      ▲
                      │
   Aday Modelin Replay Sonucunda Ürettiği Yanıt
```

Bu sayede sadece aday modelin kendi içindeki başarısı değil, mevcut production yanıtına kıyasla ne kadar değiştiği ve regresyona yol açıp açmadığı anında saptanabilir.

---

## 7. Gelecek Evaluator: LLM-as-a-Judge

### Tasarım Modeli:
İleri aşamalarda anlamsal kaliteyi (semantic quality) puanlamak için bir hakem model (Judge LLM) kullanılabilir:
```text
Baseline Response ──┐
Candidate Response ─┼──→ Judge LLM Prompt ──→ Score (1-10) & Reason
Evaluation Rubric ──┘
```

Örnek Çıktı:
```json
{
  "score": 8.5,
  "reason": "Candidate preserved all relevant technical constraints but was slightly more verbose than baseline."
}
```

### Neden MVP'ye Alınmadı? (Bilinçli Mimari Kararı):
1. **Ek Maliyet:** Her test edilen istek için ikinci bir pahalı model çağrısı gerektirir.
2. **Ek Gecikme:** Test çalıştırma süresini ikiye katlar.
3. **Deterministik Olmama:** Hakem modelin kendisi de stokastiktir ve zamanla yanıtları değişebilir.
4. **Prompt Tasarım Yükü:** Güvenilir bir rubric ve prompt tasarımı kapsamı aşırı büyütür.
5. **Önbellekleme İhtiyacı:** Yüksek maliyeti engellemek için judge yanıtlarını cache'leme altyapısı kurmak gerekir.

*Bu nedenle LLM-as-a-Judge V1.1 veya V2 yol haritasına bırakılmıştır.*
