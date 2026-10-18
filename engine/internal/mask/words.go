package mask

// The wordlist is deliberately small and has no external dependency: varied
// enough for dev seed data, deterministic, and does not add a ~50MB faker
// dataset to the sidecar binary.
var (
	firstNames = []string{
		"Budi", "Siti", "Agus", "Dewi", "Rizky", "Putri", "Andi", "Maya",
		"Fajar", "Indah", "Bayu", "Citra", "Dimas", "Nadia", "Eko", "Laras",
		"Hendra", "Mega", "Iqbal", "Ratna", "Joko", "Sari", "Kurnia", "Tari",
		"Lukman", "Wulan", "Nanda", "Yuni", "Oscar", "Ayu", "Reza", "Sinta",
	}
	lastNames = []string{
		"Santoso", "Wijaya", "Nugroho", "Pratama", "Kusuma", "Hidayat", "Saputra",
		"Ramadhan", "Setiawan", "Firmansyah", "Halim", "Gunawan", "Permata",
		"Anggraini", "Purnama", "Maulana", "Hartono", "Suryani", "Lesmana", "Iskandar",
	}
	streetNames = []string{
		"Merdeka", "Sudirman", "Diponegoro", "Gatot Subroto", "Ahmad Yani",
		"Melati", "Kenanga", "Cendrawasih", "Mangga Besar", "Pahlawan",
		"Veteran", "Kartini", "Imam Bonjol", "Cikini", "Raya Bogor",
	}
	cityNames = []string{
		"Jakarta Selatan", "Bandung", "Surabaya", "Yogyakarta", "Semarang",
		"Medan", "Denpasar", "Makassar", "Malang", "Bogor", "Tangerang", "Bekasi",
	}
	companyWords = []string{
		"Nusantara", "Cahaya", "Sentosa", "Karya", "Bakti", "Andalan", "Prima",
		"Digital", "Teknologi", "Solusi", "Mandiri", "Sejahtera", "Utama", "Global",
	}
)
