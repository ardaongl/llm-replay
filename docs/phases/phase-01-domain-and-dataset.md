# Phase 1 — Domain Modelleri ve Dataset Altyapısı

## 1. Fazın Amacı
Projenin sağlayıcılardan bağımsız temel veri modellerini (`internal/domain`) oluşturmak; JSONL formatındaki veri setlerini güvenli, bellek dostu (streaming/line-by-line) ve şema doğrulamalı şekilde okuyup yazabilen veri seti motorunu (`internal/dataset`) inşa etmek.

---

## 2. Kapsam ve Yapılacak İşler

1. **Çekirdek Domain Tiplerinin Tanımlanması (`internal/domain`):**
   * `Record`: Tekil istek-yanıt kaydı (ID, timestamp, provider, model, status vb.).
   * `Request`: Normalize edilmiş istek (messages, temperature, max_tokens vb.).
   * `Message`: Rol (`system`, `user`, `assistant`) ve metin içeriği (`content`).
   * `Response`: Model yanıt içeriği ve bitiş nedeni (`finish_reason`).
   * `RecordMetrics`: İstek seviyesi gecikme ve token sayıları.
   * `ExpectedEvaluation`: Beklenen JSON Şeması veya Exact Match değeri.
2. **Dataset Okuyucu (`Reader`) İmplementasyonu (`internal/dataset`):**
   * `bufio.Scanner` kullanarak streaming satır satır okuma (RAM dostu).
   * Her satırın JSON validasyonu ve `domain.Record` nesnesine deserileştirilmesi.
   * Hatalı satırları tespit etme ve raporlama (satır numarası ile).
3. **Dataset Yazıcı (`Writer`) İmplementasyonu (`internal/dataset`):**
   * Güvenli dosya yazma ve dosya sonuna ekleme (`os.O_APPEND|os.O_CREATE|os.O_WRONLY`).
   * Atomik yazma ve flush işlemleri.
4. **Metadata ve Hashing:**
   * Veri setinin `SHA256` özetini hesaplayan yardımcı fonksiyon.
   * `.meta.json` dosyası oluşturma ve okuma mekanizması.
5. **Birim Testleri (`internal/dataset/*_test.go`):**
   * `TestReader_ValidJSONL`: Doğru JSONL dosyasının eksiksiz okunması.
   * `TestReader_CorruptedLines`: Bozuk satır içeren dosyalarda hata yönetimi.
   * `TestWriter_AppendAndCreate`: Yeni dosya oluşturma ve satır ekleme.
   * `TestDataset_Roundtrip`: Yazılan kayıtların okunduğunda birebir aynı veriyi vermesi (Roundtrip test).

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] 1000 satırlık bir JSONL dosyası tüm belleği tüketmeden streaming olarak başarıyla okunabilmeli.
- [ ] Roundtrip testi (`Record` → `JSONL string` → `Record`) kayıpsız çalışmalı.
- [ ] Geçersiz şemaya sahip veya bozuk satırlarda uygun hata mesajları döndürülmeli.
- [ ] Veri setinin SHA256 hash değeri doğru hesaplanmalı.
