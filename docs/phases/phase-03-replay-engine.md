# Phase 3 — Replay Engine ve Worker Pool

## 1. Fazın Amacı
Dataset'ten okunan yüzlerce isteği belirlenen eşzamanlılık (concurrency) seviyesinde sağlayıcı adaptörlerine gönderen, zaman aşımlarını ve yeniden denemeleri (exponential backoff) yöneten, sonuçları disk üzerinde izole bir "Run" olarak toplayan **Replay Motorunu** (`internal/replay`) geliştirmek.

---

## 2. Kapsam ve Yapılacak İşler

1. **Worker Pool Mimarisi:**
   * Go kanalları (`jobs chan domain.Record`, `results chan ReplayResult`) ve `sync.WaitGroup` ile kontrollü eşzamanlılık.
   * CLI `--concurrency` bayrağı ile dinamik worker sayısı belirleme (Varsayılan: 5).
2. **Context ve Timeout Yönetimi:**
   * İstek bazında `context.WithTimeout(ctx, timeoutDuration)` tanımlama (Varsayılan: 30s).
   * Zaman aşımına uğrayan isteklerin `status: "timeout"` olarak işaretlenmesi ve motorun kilitlenmeden devam etmesi.
3. **Yeniden Deneme (Retry with Exponential Backoff):**
   * Yalnızca geçici hatalarda (HTTP 429, HTTP 5xx, Network Timeout) yeniden deneme.
   * `1s`, `2s`, `4s` artan süreler ve rastgele jitter ekleme.
   * Sağlayıcıdan gelen `Retry-After` HTTP başlığına öncelik verme.
   * Maksimum deneme sınırına (örn. 3) ulaşıldığında hatayı kaydetme.
4. **Run Artifact Çıktı Yönetimi:**
   * `runs/run_<timestamp>/` dizini oluşturma.
   * `config.json` dosyasına çalışma konfigürasyonunu ve dataset SHA256 özetini mühürleme.
   * `results.jsonl` dosyasına tamamlanan her isteğin aday model yanıtını ve ham metriklerini streaming olarak yazma.
5. **Entegrasyon Testleri (`internal/replay/engine_test.go`):**
   * Mock HTTP sunucusu arkasında 100 kayıtlık yapay bir veri setinin 5 worker ile eşzamanlı replaying testi.
   * Bazı isteklerin bilerek 429 ve timeout döndürdüğü dayanıklılık (resilience) senaryolarının doğrulanması.

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] 100 kayıtlık bir test dataseti `--concurrency 5` ile paralel olarak başarıyla replaying edilmeli.
- [ ] Geciken istekler zaman aşımına uğramalı, ancak worker pool kilitlenmeden diğer işleri tamamlamalı.
- [ ] `runs/<run_id>/config.json` ve `runs/<run_id>/results.jsonl` dosyaları eksiksiz oluşturulmalı.
- [ ] Yeniden denenebilir hatalar backoff kurallarına uygun olarak denenmeli, kalıcı hatalar (400, 401) denenmemeli.
