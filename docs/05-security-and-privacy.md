# 05 — Güvenlik, Gizlilik ve Ağ Politikası

## 1. Temel İlke: Local-First Gizlilik

Production ortamındaki LLM istekleri müşteri verileri, iç yazışmalar, ticari sırlar veya hassas veriler barındırabilir.

> **Güvenlik Taahhüdü:** LLM Replay **%100 local-first** (yerel öncelikli) bir araçtır.
> * Hiçbir veri harici bir sunucuya, üçüncü taraf analitik platformuna veya LLM Replay bulut servisine gönderilmez.
> * Tüm veri setleri (`.jsonl`), çalıştırma çıktıları (`runs/`) ve konfigürasyonlar tamamen kullanıcının yerel makinesinde veya kendi özel CI/CD sunucusunda barındırılır.

---

## 2. API Anahtarları ve Gizli Bilgiler (Secrets)

1. **Ortam Değişkenleri (Environment Variables):**
   * Model sağlayıcılarının API anahtarları yalnızca ortam değişkenlerinden okunmalıdır:
     * `OPENAI_API_KEY`
     * `ANTHROPIC_API_KEY`
   * Konfigürasyon dosyalarına (`llm-replay.yaml`) düz metin (plaintext) secret yazılması kesinlikle engellenmeli ve dokümanlarda önerilmemelidir.
2. **Terminal ve Log Maskeleme:**
   * CLI çıktıları, terminal tabloları ve `--verbose` logları API anahtarlarını asla tam veya kısmi olarak ekrana yazdırmaz.
   * Çıktılarda yalnızca sağlayıcı adı ve kullanılan model adı yer alır.

---

## 3. Capture Proxy Güvenliği ve Ağ İzolasyonu

Production ortamında çalışan uygulamaların trafiğini yakalayan reverse proxy bileşeni sıkı ağ kısıtlamalarına tabidir:

1. **Yalnızca Localhost Bağlantısı (Localhost-Only Bind):**
   * Proxy dinleme adresi varsayılan olarak **`127.0.0.1`** (loopback interface) üzerine bağlanır.
   * `0.0.0.0` (tüm ağ arayüzlerine açılma) varsayılan olarak kesinlikle engellenir. Kullanıcı açıkça harici IP belirtmediği sürece dış ağdan erişim imkansızdır.
2. **Doğrudan Upstream İletimi:**
   * Proxy, uygulamadan gelen isteği doğrudan ilgili sağlayıcının resmi HTTPS uç noktasına iletir (örn. `https://api.openai.com`). Araya hiçbir ara sunucu veya üçüncü parti katman girmez.

---

## 4. Veri Sanitizasyonu ve Secret Redaction (Temizleme)

Proxy üzerinden yakalanan istekler JSONL dosyasına kaydedilmeden önce zorunlu bir güvenlik filtresinden geçer:

### Minimum Zorunlu Redaction (MVP Kapsamı):
* **HTTP Headers:**
  * `Authorization` (Bearer token'lar) **kesinlikle dataset'e yazılmaz**, tamamen kaldırılır veya `[REDACTED]` ile maskelenir.
  * `Cookie` ve `Set-Cookie` başlıkları silinir.
  * `x-api-key` başlıkları silinir.
* **Gövde (Body) Güvenliği:**
  * İstek gövdesinde yer alabilecek API anahtarları loglanmaz.

### İleri Aşama: Konfigüre Edilebilir PII Redaction (V1.1+):
Geliştiricilerin belirli JSON yollarını (JSONPath) maskelemesine olanak tanıyacak genişletilebilir filtreleme yapısı:
```yaml
redact:
  headers:
    - authorization
    - cookie
  json_paths:
    - $.messages[*].content.email
    - $.user.phone_number
```
Bu yapı sayesinde GDPR, KVKK ve HIPAA uyumluluğu olan veri setleri üretilebilecektir.
