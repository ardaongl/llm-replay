# Phase 8 — Anthropic Provider Adaptörü (Cross-Provider Benchmark)

## 1. Fazın Amacı
Sisteme ikinci büyük sağlayıcı olan **Anthropic Adaptörünü** (`internal/provider/anthropic`) eklemek; sağlayıcılar arası format dönüşümünü tamamlayarak aynı production veri setini hem OpenAI hem Anthropic modellerine karşı eş zamanlı çalıştırıp gerçek bir **Cross-Provider Benchmark** deneyimi sunmak.

---

## 2. Kapsam ve Yapılacak İşler

1. **Anthropic Messages API Entegrasyonu:**
   * Uç nokta: `https://api.anthropic.com/v1/messages`
   * Zorunlu başlıklar:
     * `x-api-key: <ANTHROPIC_API_KEY>`
     * `anthropic-version: 2023-06-01`
     * `content-type: application/json`
2. **Mesaj Şeması Dönüşümü (Translation):**
   * OpenAI formatındaki `role: "system"` mesajlarını tespit edip Anthropic'in beklediği üst seviye `system: "..."` parametresine taşıma.
   * `role: "user"` ve `role: "assistant"` mesajlarını Anthropic `messages` dizisine dönüştürme.
   * `max_tokens` alanının doğrulanması (Anthropic için zorunlu alandır; istekte yoksa güvenli bir varsayılan atanır).
3. **Kullanım (Token) ve Yanıt Ayrıştırma:**
   * Yanıt gövdesindeki `content[0].text` içeriğini `domain.Response` nesnesine aktarma.
   * `usage.input_tokens` ve `usage.output_tokens` alanlarını okuyup `domain.RecordMetrics` içerisine yerleştirme.
4. **Hata Normalizasyonu:**
   * Anthropic hata tiplerini (`authentication_error`, `rate_limit_error`, `invalid_request_error`, `overloaded_error`) iç hata taksonomimize normalize etme.
5. **Entegrasyon ve Cross-Provider Testleri:**
   * Mock Anthropic sunucusu ile birim testleri.
   * Aynı dataset üzerinde bir adet OpenAI modeli ve bir adet Anthropic modelinin tek komutla çalıştırılıp karşılaştırılmasının doğrulanması:
     ```bash
     llm-replay replay dataset.jsonl \
       --model openai/gpt-4o-mini \
       --model anthropic/claude-3-5-haiku
     ```

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] Normalize edilmiş `domain.Request`, Anthropic API formatına hatasız dönüştürülebilmeli.
- [ ] System mesajları Anthropic standardına uygun biçimde ayrı parametre olarak iletilebilmeli.
- [ ] Tek bir replay koşusunda hem OpenAI hem Anthropic modelleri paralel çalışıp yan yana karşılaştırma tablosu üretebilmeli.
