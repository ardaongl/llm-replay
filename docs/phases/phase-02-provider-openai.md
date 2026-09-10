# Phase 2 — Provider Arayüzü ve OpenAI Adaptörü

## 1. Fazın Amacı
Sistemi model sağlayıcılarına bağlayan temel `Provider` arayüzünü tanımlamak, ilk ve referans sağlayıcı olarak **OpenAI Adaptörünü** hayata geçirmek, kullanım (token) metriklerini ayrıştırmak ve sağlayıcı hatalarını standart iç hata taksonomisine normalize etmek.

---

## 2. Kapsam ve Yapılacak İşler

1. **Provider Arayüzünün Tanımlanması (`internal/provider/provider.go`):**
   ```go
   type Provider interface {
       Name() string
       Generate(ctx context.Context, req domain.Request) (*domain.Response, *domain.RecordMetrics, error)
   }
   ```
2. **Hata Taksonomisi ve Normalizasyon:**
   * HTTP durum kodları ve sağlayıcı yanıt gövdelerini standart `ErrorType` enum'ına çevirme:
     * `401/403` → `ErrAuthentication`
     * `429` → `ErrRateLimit`
     * `context.DeadlineExceeded` → `ErrTimeout`
     * `5xx` → `ErrProviderDown`
     * `400` → `ErrInvalidRequest`
3. **OpenAI Adaptörü (`internal/provider/openai/`):**
   * İstek gövdesini oluşturma (`/v1/chat/completions` şeması).
   * HTTP istemcisi (özelleştirilebilir timeout ile).
   * Yetkilendirme (`Authorization: Bearer <OPENAI_API_KEY>`).
   * Yanıt gövdesinden `choices[0].message.content` ve `finish_reason` ayrıştırma.
   * `usage.prompt_tokens` ve `usage.completion_tokens` alanlarını `RecordMetrics`'e aktarma.
   * İstek süresini milisaniye cinsinden (`latency_ms`) ölçme.
4. **Mock HTTP Sunucusu ile Testler (`internal/provider/openai/adapter_test.go`):**
   * *CI ortamında asla gerçek OpenAI API çağrısı yapılmamalıdır.*
   * `httptest.Server` kullanılarak 200 OK, 429 Rate Limit, 500 Internal Error ve gecikmeli (timeout) yanıt senaryolarının test edilmesi.

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] Normalize edilmiş bir `domain.Request` nesnesi OpenAI adaptörüne verilip mock sunucuda başarıyla çalıştırılabilmeli.
- [ ] Gelen yanıt `domain.Response` ve `domain.RecordMetrics` nesnelerine doğru dönüştürülmeli.
- [ ] 429 yanıtı geldiğinde hata tipi `rate_limit` olarak normalize edilmeli.
- [ ] API Key eksik olduğunda açık ve anlaşılır bir hata döndürülmeli.
