# Phase 4 — Metrikler ve Fiyatlandırma Sistemi (Metrics & Pricing)

## 1. Fazın Amacı
Replay koşusu sırasında toplanan ham verileri aggregate ederek güvenilirlik (reliability), performans (P50/P95 gecikme), token tüketimi ve sürüm kontrollü fiyatlandırma kütüğü (`pricing registry`) üzerinden tahmini maliyet metriklerini hesaplamak; sonuçları `summary.json` dosyasına dökmek.

---

## 2. Kapsam ve Yapılacak İşler

1. **İstatistik ve Yüzdelik (Percentile) Hesaplayıcı (`internal/metrics`):**
   * Başarılı isteklerin gecikme sürelerini sıralayarak **P50** ve **P95** hesaplama algoritması.
   * Ortalama gecikme (`avg_latency_ms`) hesabı.
   * Başarı oranı (`success_rate`), hata oranı (`error_rate`) ve zaman aşımı oranı (`timeout_rate`).
   * Toplam girdi, çıktı ve toplam token agregasyonu.
2. **Fiyatlandırma Kayıt Kütüğü (`internal/pricing`):**
   * `pricing.yaml` şemasının tanımlanması ve parse edilmesi.
   * Model bazlı girdi ve çıktı birim fiyatları (`input_per_million`, `output_per_million`).
   * İstek bazlı maliyet ve tüm koşunun toplam tahmini maliyeti hesabı:
     $$\text{Cost} = \left(\frac{\text{Input Tokens}}{1,000,000} \times \text{Input Rate}\right) + \left(\frac{\text{Output Tokens}}{1,000,000} \times \text{Output Rate}\right)$$
   * Kullanıcının harici `--pricing` dosyası belirtebilmesi.
3. **Özet Rapor Oluşturucu (`summary.json`):**
   * Hesaplanan tüm metriklerin tek bir JSON dosyasında `runs/<run_id>/summary.json` olarak kaydedilmesi.
4. **Birim Testleri (`internal/metrics/*_test.go`, `internal/pricing/*_test.go`):**
   * `TestPercentile_P50_P95`: Farklı veri dağılımlarında yüzdelik hesaplama doğruluğu.
   * `TestPricing_Calculation`: Farklı token sayıları ve fiyat tarifelerinde kuruşu kuruşuna doğru maliyet çıktısı.
   * `TestMetrics_Aggregator`: Başarısız isteklerin token ve gecikme metriklerini bozmadığının doğrulanması.

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] Replay koşusu sonunda `summary.json` otomatik olarak eksiksiz üretilmeli.
- [ ] P50 ve P95 gecikme değerleri matematiksel olarak doğru hesaplanmalı.
- [ ] Belirtilen model için girdi ve çıktı tokenlarına göre tahmini maliyet tam olarak hesaplanmalı.
- [ ] Özel `--pricing` dosyası verildiğinde varsayılan fiyatların üzerine başarıyla yazılabilmeli.
