package passkey

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"golang.org/x/net/idna"
	"golang.org/x/net/publicsuffix"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrDomain     = errors.New("passkey.invalidDomain")
	ErrOrigin     = errors.New("passkey.invalidOrigin")
	ErrCeremony   = errors.New("passkey.expiredCeremony")
	ErrCredential = errors.New("passkey.invalidCredential")
	ErrName       = errors.New("passkey.invalidName")
)

type Config struct {
	ID       int `gorm:"primaryKey;autoIncrement:false"`
	Domain   string
	Revision int64
}

func (Config) TableName() string { return "clash_passkey_config" }

type Account struct {
	UserID int    `gorm:"primaryKey;autoIncrement:false"`
	Handle []byte `gorm:"uniqueIndex"`
}

func (Account) TableName() string { return "clash_passkey_accounts" }

type Credential struct {
	ID         string `json:"id" gorm:"primaryKey"`
	UserID     int    `json:"-" gorm:"index"`
	Name       string `json:"name"`
	Credential string `json:"-"`
	CreatedAt  int64  `json:"createdAt" gorm:"autoCreateTime:false"`
	LastUsedAt int64  `json:"lastUsedAt"`
}

func (Credential) TableName() string { return "clash_passkey_credentials" }

type Ceremony struct {
	ID         string `gorm:"primaryKey"`
	Binding    string `gorm:"index"`
	Kind       string
	UserID     int
	LoginEpoch int64
	Origin     string
	Revision   int64
	Session    string
	ExpiresAt  int64 `gorm:"index"`
}

func (Ceremony) TableName() string { return "clash_passkey_ceremonies" }

type View struct {
	Domain      string       `json:"domain"`
	Credentials []Credential `json:"credentials"`
}
type BeginResult struct {
	RequestID string `json:"requestId"`
	Options   any    `json:"options"`
}
type Service struct{ db *gorm.DB }

func New(db *gorm.DB) *Service { return &Service{db: db} }

func (s *Service) Migrate() error {
	if err := s.db.AutoMigrate(&Config{}, &Account{}, &Credential{}, &Ceremony{}); err != nil {
		return err
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Config{ID: 1}).Error
}

func NormalizeDomain(domain string) (string, error) {
	domain, err := idna.Lookup.ToASCII(strings.ToLower(strings.TrimSpace(domain)))
	if err != nil || domain == "" || len(domain) > 253 || net.ParseIP(domain) != nil || strings.ContainsAny(domain, "/:@?#, ") || strings.HasSuffix(domain, ".") {
		return "", ErrDomain
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrDomain
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", ErrDomain
			}
		}
	}
	if domain != "localhost" {
		if _, err = publicsuffix.EffectiveTLDPlusOne(domain); err != nil {
			return "", ErrDomain
		}
	}
	return domain, nil
}

// Taking the singleton write lock serializes configuration, registration and revocation across processes.
func (s *Service) transaction(fn func(*gorm.DB, Config) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Config{}).Where("id = ?", 1).UpdateColumn("revision", gorm.Expr("revision")).Error; err != nil {
			return err
		}
		var cfg Config
		if err := tx.First(&cfg, 1).Error; err != nil {
			return err
		}
		return fn(tx, cfg)
	})
}

func (s *Service) SetDomain(domain string) error {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}
	return s.transaction(func(tx *gorm.DB, cfg Config) error {
		if domain == cfg.Domain {
			return nil
		}
		for _, row := range []any{&Credential{}, &Ceremony{}, &Account{}} {
			if err := tx.Where("1 = 1").Delete(row).Error; err != nil {
				return err
			}
		}
		return tx.Model(&Config{}).Where("id = ?", 1).Updates(map[string]any{"domain": domain, "revision": cfg.Revision + 1}).Error
	})
}

func (s *Service) List(userID int) (View, error) {
	view := View{Credentials: []Credential{}}
	err := s.transaction(func(tx *gorm.DB, cfg Config) error {
		view.Domain = cfg.Domain
		return tx.Where("user_id = ?", userID).Order("created_at DESC").Find(&view.Credentials).Error
	})
	return view, err
}

func (s *Service) Delete(userID int, id string) error {
	return s.transaction(func(tx *gorm.DB, _ Config) error {
		result := tx.Where("user_id = ? AND id = ?", userID, id).Delete(&Credential{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCredential
		}
		return nil
	})
}

func relyingParty(cfg Config, origin string) (*webauthn.WebAuthn, error) {
	u, err := url.Parse(origin)
	if err != nil || cfg.Domain == "" || u.Hostname() != cfg.Domain || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.String() != origin {
		return nil, ErrOrigin
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (cfg.Domain == "localhost" || strings.HasSuffix(cfg.Domain, ".localhost"))) {
		return nil, ErrOrigin
	}
	return webauthn.New(&webauthn.Config{
		RPID: cfg.Domain, RPDisplayName: "3x-ui", RPOrigins: []string{origin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{ResidentKey: protocol.ResidentKeyRequirementRequired, UserVerification: protocol.VerificationRequired},
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: 5 * time.Minute},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 5 * time.Minute},
		},
	})
}

type user struct {
	model.User
	handle      []byte
	credentials []webauthn.Credential
}

func (u *user) WebAuthnID() []byte                         { return u.handle }
func (u *user) WebAuthnName() string                       { return u.Username }
func (u *user) WebAuthnDisplayName() string                { return u.Username }
func (u *user) WebAuthnCredentials() []webauthn.Credential { return u.credentials }

func loadUser(tx *gorm.DB, id int) (*user, error) {
	u := &user{}
	if err := tx.First(&u.User, id).Error; err != nil {
		return nil, err
	}
	var account Account
	if err := tx.First(&account, "user_id = ?", id).Error; err != nil {
		return nil, err
	}
	u.handle = account.Handle
	var rows []Credential
	if err := tx.Where("user_id = ?", id).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		var cred webauthn.Credential
		if err := json.Unmarshal([]byte(row.Credential), &cred); err != nil {
			return nil, err
		}
		u.credentials = append(u.credentials, cred)
	}
	return u, nil
}

func bindingHash(binding string) string {
	hash := sha256.Sum256([]byte(binding))
	return hex.EncodeToString(hash[:])
}

func saveCeremony(tx *gorm.DB, row Ceremony, session *webauthn.SessionData, binding string, options any) (*BeginResult, error) {
	if binding == "" {
		return nil, ErrCeremony
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		return nil, err
	}
	row.ID = rand.Text()
	row.Binding = bindingHash(binding)
	row.Session = string(encoded)
	row.ExpiresAt = time.Now().Add(5 * time.Minute).UnixMilli()
	if err = tx.Where("expires_at < ? OR (binding = ? AND kind = ?)", time.Now().UnixMilli(), row.Binding, row.Kind).Delete(&Ceremony{}).Error; err != nil {
		return nil, err
	}
	if err = tx.Create(&row).Error; err != nil {
		return nil, err
	}
	return &BeginResult{RequestID: row.ID, Options: options}, nil
}

func (s *Service) BeginRegistration(userID int, origin, binding string) (*BeginResult, error) {
	var out *BeginResult
	err := s.transaction(func(tx *gorm.DB, cfg Config) error {
		wa, err := relyingParty(cfg, origin)
		if err != nil {
			return err
		}
		handle := make([]byte, 32)
		if _, err = rand.Read(handle); err != nil {
			return err
		}
		if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Account{UserID: userID, Handle: handle}).Error; err != nil {
			return err
		}
		u, err := loadUser(tx, userID)
		if err != nil {
			return err
		}
		exclusions := make([]protocol.CredentialDescriptor, 0, len(u.credentials))
		for _, cred := range u.credentials {
			exclusions = append(exclusions, cred.Descriptor())
		}
		options, session, err := wa.BeginRegistration(u, webauthn.WithExclusions(exclusions), webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired))
		if err != nil {
			return err
		}
		out, err = saveCeremony(tx, Ceremony{Kind: "register", UserID: userID, LoginEpoch: u.LoginEpoch, Origin: origin, Revision: cfg.Revision}, session, binding, options)
		return err
	})
	return out, err
}

func (s *Service) BeginLogin(origin, binding string) (*BeginResult, error) {
	var out *BeginResult
	err := s.transaction(func(tx *gorm.DB, cfg Config) error {
		wa, err := relyingParty(cfg, origin)
		if err != nil {
			return err
		}
		options, session, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return err
		}
		out, err = saveCeremony(tx, Ceremony{Kind: "login", Origin: origin, Revision: cfg.Revision}, session, binding, options)
		return err
	})
	return out, err
}

// Consumption commits before validation so even a failed response cannot replay its challenge.
func (s *Service) consume(id, binding, kind string, userID int) (Ceremony, error) {
	var row Ceremony
	err := s.transaction(func(tx *gorm.DB, _ Config) error {
		if err := tx.Where("id = ? AND binding = ? AND kind = ? AND user_id = ?", id, bindingHash(binding), kind, userID).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCeremony
			}
			return err
		}
		return tx.Delete(&row).Error
	})
	if err == nil && row.ExpiresAt <= time.Now().UnixMilli() {
		err = ErrCeremony
	}
	return row, err
}

func ceremonyParty(cfg Config, row Ceremony, origin string) (*webauthn.WebAuthn, webauthn.SessionData, error) {
	var session webauthn.SessionData
	if cfg.Revision != row.Revision || row.Origin != origin {
		return nil, session, ErrCeremony
	}
	if err := json.Unmarshal([]byte(row.Session), &session); err != nil {
		return nil, session, err
	}
	wa, err := relyingParty(cfg, origin)
	return wa, session, err
}

func (s *Service) FinishRegistration(userID int, origin, binding, id, name string, response []byte) error {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return ErrName
	}
	row, err := s.consume(id, binding, "register", userID)
	if err != nil {
		return err
	}
	return s.transaction(func(tx *gorm.DB, cfg Config) error {
		wa, session, err := ceremonyParty(cfg, row, origin)
		if err != nil {
			return err
		}
		u, err := loadUser(tx, userID)
		if err != nil {
			return err
		}
		if row.LoginEpoch != u.LoginEpoch {
			return ErrCeremony
		}
		parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
		if err != nil {
			return ErrCredential
		}
		cred, err := wa.CreateCredential(u, session, parsed)
		if err != nil {
			return ErrCredential
		}
		encoded, err := json.Marshal(cred)
		if err != nil {
			return err
		}
		return tx.Create(&Credential{ID: base64.RawURLEncoding.EncodeToString(cred.ID), UserID: userID, Name: name, Credential: string(encoded), CreatedAt: time.Now().UnixMilli()}).Error
	})
}

func (s *Service) FinishLogin(origin, binding, id string, response []byte) (*model.User, error) {
	row, err := s.consume(id, binding, "login", 0)
	if err != nil {
		return nil, err
	}
	var result *model.User
	err = s.transaction(func(tx *gorm.DB, cfg Config) error {
		wa, session, err := ceremonyParty(cfg, row, origin)
		if err != nil {
			return err
		}
		parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
		if err != nil {
			return ErrCredential
		}
		handler := func(rawID, handle []byte) (webauthn.User, error) {
			var account Account
			if err := tx.Where("handle = ?", handle).First(&account).Error; err != nil {
				return nil, ErrCredential
			}
			u, err := loadUser(tx, account.UserID)
			if err != nil {
				return nil, ErrCredential
			}
			for _, credential := range u.credentials {
				if bytes.Equal(rawID, credential.ID) {
					return u, nil
				}
			}
			return nil, ErrCredential
		}
		validated, cred, err := wa.ValidatePasskeyLogin(handler, session, parsed)
		if err != nil || cred.Authenticator.CloneWarning {
			return ErrCredential
		}
		u := validated.(*user)
		encoded, err := json.Marshal(cred)
		if err != nil {
			return err
		}
		updated := tx.Model(&Credential{}).Where("id = ? AND user_id = ?", base64.RawURLEncoding.EncodeToString(cred.ID), u.Id).Updates(map[string]any{"credential": string(encoded), "last_used_at": time.Now().UnixMilli()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrCredential
		}
		result = &u.User
		return nil
	})
	return result, err
}
