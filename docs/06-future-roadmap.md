# 06 — Gelecek Yol Haritası ve İleri Özellikler

## 1. MVP Sonrası Öncelik Sıralaması

MVP sürümünün başarıyla yayınlanmasının ardından sisteme eklenecek özelliklerin sıralaması:

```text
1. CI Regresyon Eşikleri (llm-replay check)
2. HTML Görsel Karşılaştırma Raporu
3. LLM-as-a-Judge Anlamsal Değerlendirici
4. Prompt Varyantları (Prompt v1 vs Prompt v2 Benchmark)
5. Dataset Örnekleme (Sampling) ve Filtreleme (Filtering)
6. Streaming (Akış) Desteği ve Passthrough Capture
7. Tool Calling / Function Calling Desteği
8. OpenTelemetry Dağıtık İzleme (Distributed Tracing)
9. Ek Model Sağlayıcıları (Google Gemini, Mistral, Ollama vb.)
10. Gelişmiş PII Anonimizasyonu ve Maskeleme
```

---

## 2. CI/CD Entegrasyonu ve Regresyon Testleri (`llm-replay check`)

Projenin uzun vadeli vizyonundaki en güçlü kullanım senaryolarından biri: **"LLM sistemleri için pytest"** olmaktır.

Bir geliştirici Pull Request (PR) açtığında veya promptu güncellediğinde, CI pipeline'ı otomatik olarak regresyon veri setini çalıştırır:
```bash
llm-replay check evals/regression.jsonl --thresholds ci-thresholds.yaml
```

### Örnek Eşik Konfigürasyonu (`ci-thresholds.yaml`):
```yaml
thresholds:
  success_rate:
    min: 0.99           # Başarı oranı %99'un altına düşerse CI patlasın
  json_valid:
    min: 0.98           # JSON geçerliliği %98'in altındaysa patlasın
  cost:
    max_increase: 0.10  # Maliyet %10'dan fazla artarsa patlasın
  latency_p95_ms:
    max: 2500           # P95 gecikmesi 2.5 saniyeyi geçerse patlasın
```

### Örnek CI Terminal Çıktısı:
```text
Running Regression Test: evals/regression.jsonl (120 cases)
Evaluating thresholds...

  [PASS] Success Rate     : 99.2% (Threshold: >= 99.0%)
  [FAIL] JSON Validity    : 95.8% (Threshold: >= 98.0%)
  [PASS] Cost Change      : -12.4% (Threshold: <= +10.0%)

FAILED CASES:
  #42: Schema adherence failed (missing required key 'urgency')
  #87: Invalid JSON formatting (trailing comma)

Process exited with status 1.
```

---

## 3. Yanıt Karşılaştırma ve Fark Görünümü (Diff View)

Mevcut baseline yanıtı ile aday modelin ürettiği yanıt arasındaki farkları satır satır gösteren CLI ve HTML diff arayüzü:
```text
Baseline (GPT-4o):
The user request should be escalated to the billing department immediately.

Candidate (Claude-3.5-Haiku):
The user inquiry has been routed to the payment support team with high priority.
```

---

## 4. Örnekleme (Sampling) ve Filtreleme (Filtering)

Production veri setleri yüz binlerce veya milyonlarca kayıt içerebilir. Tüm veriyi replay etmek maliyetli ve yavaş olacağından:
* **Sampling:**
  ```bash
  llm-replay replay data.jsonl --model model-b --sample 100
  # veya yüzdelik örnekleme:
  llm-replay replay data.jsonl --model model-b --sample-rate 0.05
  ```
* **Filtering:**
  ```bash
  llm-replay replay data.jsonl --model model-b --filter "status=success"
  ```

---

## 5. Prompt Varyantları (Prompt Testing)

Yalnızca model değişikliği değil, aynı model üzerinde iki farklı prompt versiyonunu karşılaştırma:
```yaml
candidates:
  - name: current-prompt
    model: openai/gpt-4o-mini
    prompt_template: prompts/system_v1.txt
  - name: new-concise-prompt
    model: openai/gpt-4o-mini
    prompt_template: prompts/system_v2.txt
```

---

## 6. Provider Migration Araç Kiti

Forward Deployed Engineer (FDE) ve kurumsal ekipler için hazır "OpenAI'dan Anthropic'e veya Açık Kaynak Modellere Geçiş Raporu" üreten analiz modu:
* Token oranı farklılıkları
* Formatlama uyumluluğu
* Tahmini yıllık fatura tasarrufu / ek maliyeti

---

## 7. Tool Calling / Function Calling Desteği

* Modelin ürettiği tool çağrılarının şemaya uygunluğu (`tool_calls.function.arguments`).
* Beklenen tool'un çağrılıp çağrılmadığının evaluation kurallarıyla test edilmesi.

---

## 8. Streaming Capture & Passthrough

* `stream: true` olan isteklerin Server-Sent Events (SSE) akışı bozulmadan istemciye iletilmesi.
* Akış tamamlandığında tüm parçaların (chunks) birleştirilerek tek bir `Record` halinde JSONL'e kaydedilmesi.

---

## 9. SQLite Run Kataloğu ve İndeksleme

* Çok sayıda çalıştırma (yüzlerce run) biriktiğinde geçmiş sonuçları sorgulamak (`llm-replay list-runs`, `llm-replay find --metric latency < 1000`) için yerel, hafif bir SQLite indeksi.

---

## 10. Projenin Kimliğini Koruma Sınırı (Kritik Uyarı)

İleri özellikler geliştirilirken projenin asla şunlara evrilmesine izin verilmeyecektir:
* Bir **LangSmith** veya **Langfuse** klonuna dönüşmemelidir.
* Bir LLM observability platformu haline gelmemelidir.
* Projenin odağı her zaman: **"Replay tabanlı regresyon testi ve model karşılaştırma"** olarak kalacaktır.
