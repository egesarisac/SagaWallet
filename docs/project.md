🚀 Go-Fintech: Dağıtık Cüzdan Sistemi (Saga Pattern ile)
Bu proje, yüksek performanslı, ölçeklenebilir ve tutarlı bir dijital cüzdan altyapısını mikroservis mimarisiyle simüle etmeyi hedefler.

🏗 Mimari Genel Bakış
Sistem üç ana mikroservisten oluşur ve Saga Choreography desenini kullanarak servisler arası tutarlılığı sağlar.

Wallet Service (Go + Gin + sqlc): Bakiyeleri yönetir.

Transaction Service (Go + Gin + sqlc): Tüm işlem geçmişini tutar ve transfer süreçlerini orkestre eder.

Notification Service (Go): Kullanıcılara işlem sonuçlarını asenkron olarak bildirir.

🛠 Teknoloji Yığını (Tech Stack)
Bileşen	Seçilen Teknoloji	Neden?
Dil	Go (Golang)	Eşzamanlılık (Concurrency) ve düşük gecikme.
Web Framework	Gin	Hızlı, hafif ve geniş topluluk desteği.
DB / Data Layer	PostgreSQL + sqlc	SQL yazarak tip güvenli Go kodları üretmek için en iyi yöntem.
İç İletişim	gRPC (Protocol Buffers)	Servisler arası hızlı ve şemalı iletişim.
Asenkron Mesajlaşma	Kafka (veya Redpanda)	Saga Pattern ve event-driven mimari için standart.
Altyapı (IaC)	Terraform + AWS	Bulut kaynaklarını kodla yönetmek için.
Konfigürasyon	Viper	Ortam değişkenlerini (Env) kolay yönetmek için.
📁 Proje Yapısı (Folder Layout)
go-fintech/
├── api/                    # gRPC proto tanımları (.proto)
├── build/                  # Dockerfile ve deployment scriptleri
├── deployments/            # Terraform (AWS) dosyaları
├── services/
│   ├── wallet-service/
│   │   ├── cmd/            # Uygulama giriş noktası (main.go)
│   │   ├── internal/
│   │   │   ├── db/         # sqlc tarafından üretilen kodlar
│   │   │   ├── handler/    # HTTP/Gin handlerları
│   │   │   ├── service/    # Business Logic
│   │   │   └── repository/ # sqlc sorguları ve DB katmanı
│   │   ├── schema/         # SQL şemaları ve sqlc.yaml
│   │   └── go.mod
│   ├── transaction-service/
│   └── notification-service/
├── docker-compose.yml
└── README.md
🔄 Saga Flow: Para Transferi (Transfer Money)
Bir kullanıcıdan diğerine para gönderilirken izlenecek yol:

Transaction Service: Bir transfer kaydı oluşturur (Durum: PENDING).

Kafka Event: MoneyReserved event'i yayınlanır.

Wallet Service: Gönderen kişinin bakiyesini kontrol eder ve düşer (Debit).

Kafka Event: İşlem başarılıysa DebitSuccess, başarısızsa DebitFailed yayınlanır.

Transaction Service: * Eğer DebitSuccess gelirse; alıcıya para eklenmesi için emir verir.

Eğer DebitFailed gelirse; transferi iptal eder (CANCELLED).