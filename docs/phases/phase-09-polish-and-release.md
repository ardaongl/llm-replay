# Phase 9 — Vitrin, Demo, Dokümantasyon ve Açık Kaynak Yayını (Polish & Release)

## 1. Fazın Amacı
Tüm MVP özelliklerini yüksek kaliteli bir **README**, hazır çalışan bir **Demo Veri Seti (Fixture)**, net mimari diyagramları ve otomatik GitHub Actions CI/CD süreci ile taçlandırmak; projeyi GitHub'da dünya standartlarında bir açık kaynak AI altyapı aracı olarak konumlandırmak.

---

## 2. Kapsam ve Yapılacak İşler

1. **Örnek Demo Veri Seti ve Şeması (`examples/`):**
   * `examples/datasets/support.jsonl`: 100-200 adet gerçekçi müşteri destek sınıflandırma isteği (faturalandırma, teknik destek, üyelik iptali vb.).
   * `examples/schemas/support_schema.json`: Destek talebi JSON çıktısının beklenen şeması (`category`, `urgency` alanları zorunlu).
2. **Kusursuz README.md Tasarımı:**
   * **Tagline & Hero Bölümü:**
     > *"Before switching your production LLM: capture → replay → compare"*
   * **Demo Terminal Önizlemesi:** Renkli ASCII veya animasyonlu terminal karşılaştırma çıktısı.
   * **Neden LLM Replay? (Why?):** Manuel prompt testlerinin riskleri ve production iş yükü ihtiyacı.
   * **5 Dakikada Hızlı Başlangıç (Quick Start):**
     ```bash
     git clone https://github.com/ardao/llm-replay.git
     cd llm-replay
     make build
     export OPENAI_API_KEY="your-key"
     ./bin/llm-replay replay examples/datasets/support.jsonl --model openai/gpt-4o-mini
     ```
   * **Mimari Diyagramı:** Temiz ASCII veya Mermaid mimari şeması.
   * **Gizlilik Taahhüdü (Privacy by Default):** Local-first felsefesi ve secret redaction garantisi.
   * **Yol Haritası (Roadmap):** Gelecek sürümler ve katkı kılavuzu (`CONTRIBUTING.md`).
3. **5 Dakika Kuralının Doğrulanması (5-Minute Smoke Test):**
   * Temiz bir ortamda reponun klonlanması, derlenmesi ve hazır örnek veri seti ile benchmark'ın sıfır pürüzle tamamlandığının test edilmesi.
4. **Dağıtım ve Sürümleme Hazırlığı:**
   * GoReleaser konfigürasyonu (`.goreleaser.yaml`) ile Linux, macOS (Intel & Apple Silicon) ve Windows binary derlemeleri.
   * GitHub Release workflow'u.

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] Yeni bir geliştirici repoyu klonladıktan sonra 5 dakika içinde tek bir hata almadan örnek demoyu çalıştırıp karşılaştırma tablosunu görebilmeli.
- [ ] README dosyası görsel olarak etkileyici, açık, net ve profesyonel olmalı.
- [ ] `examples/datasets/support.jsonl` dosyası bozuk veya eksik satır içermemeli.
- [ ] CI pipeline'ı testleri, linter kontrollerini ve derleme adımlarını yeşile çekmeli.
- [ ] Proje, hem FDE (Forward Deployed Engineer) hem de AI Backend Engineer rollerinde üst düzey mühendislik kalitesini kanıtlayacak olgunlukta olmalı.
