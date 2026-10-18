package introspect

import "strings"

// piiPattern is a column-name -> PII-category heuristic rule. The goal is not
// perfect classification but 80% of the value: by the time the user opens
// Step 3, sensitive columns are already flagged and a strategy suggestion is
// filled in, so all they have to do is approve it.
type piiPattern struct {
	category   string
	exact      []string
	contains   []string
	confidence float64
}

var piiPatterns = []piiPattern{
	{category: "email", exact: []string{"email", "email_address", "mail", "e_mail", "user_email"}, contains: []string{"email", "mail"}, confidence: 0.95},
	{category: "password", exact: []string{"password", "passwd", "pwd", "password_hash", "hashed_password", "pass_hash", "passphrase", "secret", "api_key", "apikey", "access_token", "refresh_token", "private_key", "otp_secret", "totp_secret"}, contains: []string{"password", "passwd", "pwd", "secret", "token", "api_key", "apikey"}, confidence: 0.92},
	{category: "phone", exact: []string{"phone", "phone_number", "mobile", "mobile_number", "telp", "telepon", "no_hp", "nomor_hp", "handphone", "wa", "whatsapp", "msisdn", "contact_number"}, contains: []string{"phone", "mobile", "telp", "telepon", "hp", "msisdn"}, confidence: 0.88},
	{category: "national_id", exact: []string{"nik", "ktp", "ssn", "nid", "national_id", "identity_number", "passport_number", "npwp", "sim"}, contains: []string{"nik", "ktp", "ssn", "passport"}, confidence: 0.85},
	{category: "credit_card", exact: []string{"credit_card", "card_number", "cc_number", "card_no", "pan", "cvv", "cvc", "iban"}, contains: []string{"card_number", "credit_card", "cvv", "cc_num"}, confidence: 0.9},
	{category: "person_name", exact: []string{"name", "full_name", "fullname", "first_name", "last_name", "display_name", "nama", "nama_lengkap", "username", "nickname"}, contains: []string{"_name", "nama", "firstname", "lastname"}, confidence: 0.7},
	{category: "address", exact: []string{"address", "address_line", "street", "alamat", "city", "kota", "postal_code", "zip", "zipcode", "kode_pos", "province", "provinsi", "kecamatan", "kelurahan"}, contains: []string{"address", "alamat", "street"}, confidence: 0.78},
	{category: "date_of_birth", exact: []string{"dob", "date_of_birth", "birth_date", "birthdate", "tanggal_lahir", "tgl_lahir"}, contains: []string{"birth", "lahir"}, confidence: 0.85},
	{category: "geo", exact: []string{"lat", "latitude", "lng", "lon", "longitude", "coordinates", "geolocation"}, contains: []string{"latitude", "longitude"}, confidence: 0.75},
	{category: "ip", exact: []string{"ip", "ip_address", "ipaddress", "remote_addr", "client_ip", "last_login_ip"}, contains: []string{"ip_address"}, confidence: 0.7},
	{category: "user_agent", exact: []string{"user_agent", "useragent", "device_id", "fingerprint"}, contains: []string{"user_agent"}, confidence: 0.6},
	{category: "free_text", exact: []string{"bio", "about", "description", "notes", "message", "comment", "catatan", "keterangan"}, contains: []string{}, confidence: 0.45},
}

// GuessPII returns (category, confidence). An empty category means not
// sensitive.
func GuessPII(columnName, dataType string) (string, float64) {
	n := strings.ToLower(strings.TrimSpace(columnName))
	if n == "" {
		return "", 0
	}
	// Free-text-typed columns are more relevant to this heuristic.
	textual := isTextual(dataType)

	for _, p := range piiPatterns {
		for _, e := range p.exact {
			if n == e {
				return p.category, p.confidence
			}
		}
		for _, c := range p.contains {
			if strings.Contains(n, c) {
				conf := p.confidence - 0.15
				if !textual && p.category != "date_of_birth" && p.category != "geo" {
					conf -= 0.1
				}
				return p.category, round2(conf)
			}
		}
	}
	return "", 0
}

func isTextual(dataType string) bool {
	d := strings.ToLower(dataType)
	for _, t := range []string{"char", "text", "clob", "varchar", "string", "nvarchar", "json", "uuid"} {
		if strings.Contains(d, t) {
			return true
		}
	}
	return false
}

func round2(f float64) float64 {
	if f < 0 {
		return 0
	}
	return float64(int(f*100+0.5)) / 100
}

// SuggestStrategy maps a PII category to the default masking strategy.
func SuggestStrategy(category string) string {
	switch category {
	case "email":
		return "fake_email"
	case "password":
		return "default_hash"
	case "phone":
		return "fake_phone"
	case "person_name":
		return "fake_name"
	case "national_id", "credit_card":
		return "redact"
	case "address", "geo":
		return "scramble"
	case "date_of_birth":
		return "scramble"
	case "ip", "user_agent":
		return "nullify"
	case "free_text":
		return "scramble"
	default:
		return ""
	}
}
