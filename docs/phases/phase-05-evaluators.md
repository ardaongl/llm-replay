# Phase 5 — Modüler Evaluator Sistemi (Structured Evaluators)

## 1. Fazın Amacı
Model çıktılarının yapısal doğruluğunu, şema uyumunu ve içerik tutarlılığını test eden modüler `Evaluator` mimarisini (`internal/evaluation`) kurmak; JSON Validity, JSON Schema Adherence ve Exact Match değerlendiricilerini hayata geçirmek.

---

## 2. Kapsam ve Yapılacak İşler

1. **Evaluator Arayüzü ve Veri Modelleri:**
   ```go
   type EvaluationResult struct {
       Name    string                 `json:"name"`
       Passed  bool                   `json:"passed"`
       Score   float64                `json:"score,omitempty"`
       Details map[string]interface{} `json:"details,omitempty"`
       Error   string                 `json:"error,omitempty"`
   }

   type Evaluator interface {
       Name() string
       Evaluate(ctx context.Context, record *domain.Record, candidateResp *domain.Response) EvaluationResult
   }
   ```
2. **Çekirdek Evaluator'ların Geliştirilmesi:**
   * **JSON Validity Evaluator (`json_valid`):**
     * Model çıktısının (`candidateResp.Content`) geçerli bir JSON olup olmadığını doğrular (`json.Valid`). Boş veya bozuk JSON'larda `Passed: false`.
   * **JSON Schema Adherence Evaluator (`schema_adherence`):**
     * `record.Evaluation.Schema` tanımlıysa çalışır.
     * Hafif ve hızlı bir JSON Schema doğrulayıcısı ile (örn. `xeipuuv/gojsonschema` veya `santhosh-tekuri/jsonschema`) model çıktısının şemadaki `required`, `type` vb. kurallara uyumunu test eder.
   * **Exact Match Evaluator (`exact_match`):**
     * `record.Evaluation.Expected` tanımlıysa çalışır.
     * Model çıktısı ile beklenen değeri karşılaştırır (opsiyonel boşluk ve harf kırpma desteği ile).
3. **Sonuçların Entegrasyonu:**
   * Her istek için evaluator sonuçlarının `results.jsonl` içindeki `evaluations` nesnesine yazılması.
   * Koşu genelinde `json_valid_rate`, `schema_adherence_rate`, `exact_match_rate` yüzdelerinin `summary.json`'a aggregate edilmesi.
4. **Birim Testleri (`internal/evaluation/*_test.go`):**
   * Geçerli, geçersiz, kırpılmış JSON örneklerinin test edilmesi.
   * Farklı JSON şemaları (eksik alan, yanlış tip) ile doğrulama testleri.
   * Exact match pozitif ve negatif senaryoları.

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] Her bir record için evaluation sonuçları `results.jsonl` içinde açıkça listelenmeli.
- [ ] `summary.json` dosyası `json_valid_rate`, `schema_adherence_rate` ve `exact_match_rate` oranlarını doğru hesaplamalı.
- [ ] Şema içermeyen veya evaluation tanımlanmamış kayıtlarda evaluator'lar sistemi patlatmadan güvenle atlanmalı (graceful skip).
