# Phase 6 — CLI Arayüzü ve Terminal UX (Cobra & Pretty Reports)

## 1. Fazın Amacı
Geliştiricinin tüm sistemi komut satırından rahatlıkla yönetebilmesini sağlamak; `inspect`, `replay` ve `compare` komutlarını hayata geçirmek; terminalde modern, okunaklı, renkli ve karşılaştırmalı ASCII tabloları (`internal/report`) sunmak.

---

## 2. Kapsam ve Yapılacak İşler

1. **Cobra Komut Hiyerarşisinin Tamamlanması (`cmd/llm-replay/`):**
   * `inspectCmd`: Veri setinin dosya boyutu, istek sayısı, token dağılımları ve şema varlığını analiz eder.
   * `replayCmd`: Belirtilen dataset dosyasını aday modele/modellere karşı çalıştırır.
   * `compareCmd`: Geçmiş iki çalıştırmayı veya tek bir çalıştırmadaki çoklu modelleri yan yana tablolar.
2. **Terminal Raporlama Motoru (`internal/report`):**
   * Replay sırasında anlık ilerleme çubuğu (Progress bar).
   * Çalışma bittiğinde temiz, hizalı terminal tablosu (Success rate, JSON valid %, Latency P50/P95, Tokens, Cost).
   * Terminal renkleri (başarılı olanlar yeşil, hatalar kırmızı, nötr metrikler mavi/cyan).
3. **`inspect` Komutu Detayları:**
   * Veri setini baştan sona tarar.
   * Girdi ve çıktı tokenlarının P50, P95, Max değerlerini hesaplar.
   * Kaç kayıtta `evaluation` veya JSON formatı olduğunu listeler.
4. **`compare` Komutu Detayları:**
   * `runs/run_a/summary.json` ve `runs/run_b/summary.json` dosyalarını okur.
   * İki model arasındaki metrik farklarını hesaplayıp yüzde değişimlerini (+/- %) gösterir.
5. **Kullanıcı Deneyimi Testleri:**
   * Hatalı bayrak girişlerinde açıklayıcı ve temiz hata mesajları.
   * API anahtarı bulunamadığında kullanıcıyı yönlendiren net terminal mesajı.

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] `llm-replay inspect dataset.jsonl` komutu veri seti analizini anında ekrana basmalı.
- [ ] `llm-replay replay dataset.jsonl --model openai/gpt-4o-mini` komutu uçtan uca çalışıp terminalde özet tablosunu göstermeli ve `runs/` klasörüne çıktıları yazmalı.
- [ ] `llm-replay compare runs/run_1` komutu model karşılaştırmasını okunabilir bir tablo olarak sunmalı.
- [ ] Terminal çıktıları görsel olarak profesyonel ve hizalı olmalı.
