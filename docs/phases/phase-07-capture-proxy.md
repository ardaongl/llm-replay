# Phase 7 — Capture Proxy ve İstek Yakalama (Reverse Proxy & Sanitization)

## 1. Fazın Amacı
Uygulama ile upstream LLM sağlayıcısı (OpenAI) arasına şeffaf bir şekilde girerek canlı üretim trafiğini yakalayan, hassas başlıkları (API Key, Bearer Token) temizleyen ve istek-yanıt çiftlerini yerel bir JSONL veri setine kaydeden **HTTP Reverse Proxy** bileşenini (`internal/capture`) geliştirmek.

---

## 2. Kapsam ve Yapılacak İşler

1. **HTTP Reverse Proxy Sunucusu (`internal/capture/proxy.go`):**
   * Go'nun standart `net/http/httputil.ReverseProxy` altyapısı üzerine inşa edilir.
   * Yalnızca güvenli yerel ağa (`127.0.0.1`) bağlanma kuralı (Localhost bind).
   * Dinlenecek port (`--listen :8787`) ve upstream adresi (`--upstream https://api.openai.com`) parametreleri.
2. **İstek ve Yanıt Yakalama (Traffic Interception):**
   * İstek gövdesini (request body) okuyup klonlama ve upstream'e eksiksiz iletme.
   * Giden isteğin başlangıç zamanını (`time.Now()`) ve upstream'den gelen yanıtın süresini (`latency_ms`) kaydetme.
   * Upstream yanıt gövdesini (response body) okuyup istemciye (uygulamaya) anında aktarma.
3. **Gizlilik ve Secret Redaction:**
   * Gelen istekteki `Authorization`, `Cookie`, `x-api-key` başlıklarını kayıt verisinden tamamen temizleme.
   * Yakalanan veriyi `domain.Record` formatına dönüştürme.
4. **Veri Setine Yazma (Append-Only Streaming Writer):**
   * Yakalanan her kaydı belirlenen `--output` JSONL dosyasına güvenle ekleme (`writer.Append(record)`).
5. **`llm-replay capture` CLI Komutunun Bağlanması:**
   * Komut satırından proxy'yi başlatma ve gracefully kapatma (`os.Interrupt`, `SIGTERM` dinleyerek temiz kapanış).
6. **Entegrasyon Testleri (`internal/capture/proxy_test.go`):**
   * Mock upstream sunucusu kurularak istemciden proxy'ye istek gönderilmesi.
   * Upstream yanıtının istemciye eksiksiz iletildiğinin doğrulanması.
   * Çıktı JSONL dosyasında `Authorization` başlığının bulunmadığının doğrulanması.

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] `llm-replay capture --listen :8787 --output ./datasets/captured.jsonl` komutuyla proxy ayağa kalkmalı.
- [ ] İstemci `http://localhost:8787/v1/chat/completions` adresine istek attığında yanıtı kesintisiz almalı.
- [ ] İstek ve yanıt `datasets/captured.jsonl` dosyasına geçerli bir `Record` satırı olarak eklenmeli.
- [ ] Kaydedilen JSONL kaydında API Key veya gizli token'lar kesinlikle yer almamalıdır.
