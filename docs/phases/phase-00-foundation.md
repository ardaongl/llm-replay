# Phase 0 — Repository Foundation (Temel İskelet)

## 1. Fazın Amacı
Projenin derlenebilir, test edilebilir ve standartlara uygun temel Go altyapısını oluşturmak. Bu aşamada henüz harici LLM sağlayıcı çağrısı yapılmaz; amaç temiz ve sağlam bir iskelet kurmaktır.

---

## 2. Kapsam ve Yapılacak İşler

1. **Go Modül Başlatma:**
   * Go modül adının belirlenmesi: `github.com/ardao/llm-replay` (veya belirlenen repo URL'i).
   * Go sürümünün `go 1.22` veya üzeri olarak ayarlanması.
2. **Dizin Yapısının Oluşturulması:**
   * `cmd/llm-replay/`
   * `internal/` (domain, config vb.)
   * `examples/`
   * `testdata/`
   * `.github/workflows/`
3. **Cobra CLI Giriş Noktası:**
   * `cmd/llm-replay/main.go` oluşturulması.
   * `rootCmd` tanımlanması, `--version`, `--help` ve `--verbose` bayraklarının bağlanması.
4. **Temel Konfigürasyon Yapısı (`internal/config`):**
   * Config struct yapısının oluşturulması.
   * CLI bayrakları ve ortam değişkenlerini bağlama mantığı.
5. **Makefile ve Geliştirici Betikleri:**
   * `make build` (`go build -o bin/llm-replay ./cmd/llm-replay`)
   * `make test` (`go test -v -race ./...`)
   * `make lint` (`golangci-lint run`)
   * `make clean`
6. **Dockerfile:**
   * Minimal, multi-stage, scratch veya alpine tabanlı derleme dosyası.
7. **GitHub Actions CI Pipeline (`.github/workflows/ci.yml`):**
   * Go kurulumu, testlerin koşulması (`go test -race ./...`), kod biçimlendirme kontrolü (`gofmt`).
8. **Lisans ve README İskeleti:**
   * MIT veya Apache 2.0 lisansı.
   * Giriş seviyesinde `README.md`.

---

## 3. Kabul Kriterleri (Acceptance Criteria)

- [ ] `go build ./cmd/llm-replay` komutu sıfır hata ile çalışmalı ve `llm-replay` çalıştırılabilir ikili dosyasını (binary) üretmeli.
- [ ] `./llm-replay --help` komutu temiz bir açıklama ve versiyon bilgisi basmalı.
- [ ] `go test ./...` komutu başarıyla geçmeli.
- [ ] `make build` ve `make test` komutları yerel ortamda sorunsuz çalışmalı.
- [ ] GitHub Actions workflow dosyası sözdizimsel olarak geçerli olmalı.
