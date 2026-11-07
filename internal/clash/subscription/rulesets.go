package subscription

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/netip"
	"strings"
	"time"
	"unicode"

	"github.com/dlclark/regexp2"
	"github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
	"golang.org/x/net/idna"
	"gorm.io/gorm"
)

type RuleSet struct {
	UpdatedAt *time.Time `json:"updatedAt"`
	Code      *string    `json:"code" gorm:"uniqueIndex"`
	Filename  string     `json:"filename"`
	ID        uint       `json:"id"`
	Name      string     `json:"name" gorm:"uniqueIndex;not null"`
	Routing   bool       `json:"routing"`
	Blacklist bool       `json:"blacklist"`
	Rules     []string   `json:"rules" gorm:"serializer:json"`
}

func (RuleSet) TableName() string { return "clash_rule_sets" }

// Allocate uniformly from unused four-digit codes; the unique index enforces uniqueness.
func assignPublication(tx *gorm.DB, row *RuleSet) error {
	var used []string
	if err := tx.Model(&RuleSet{}).Where("code IS NOT NULL").Pluck("code", &used).Error; err != nil {
		return err
	}
	taken := make(map[string]bool, len(used))
	for _, s := range used {
		taken[s] = true
	}
	remaining := 10000 - len(taken)
	if remaining <= 0 {
		return errors.New("四位发布码已用完")
	}
	rank, err := rand.Int(rand.Reader, big.NewInt(int64(remaining)))
	if err != nil {
		return err
	}
	n := rank.Int64()
	for i := 0; i < 10000; i++ {
		s := fmt.Sprintf("%04d", i)
		if taken[s] {
			continue
		}
		if n == 0 {
			row.Code = &s
			break
		}
		n--
	}
	row.Filename = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune("/\\:*?\"<>|%", r) {
			return '_'
		}
		return r
	}, row.Name) + ".yaml"
	return nil
}

func MigratePublications(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var rows []RuleSet
		if err := tx.Where("code IS NULL").Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			r := &rows[i]
			if err := assignPublication(tx, r); err != nil {
				return err
			}
			if err := tx.Model(r).Select("Code", "Filename").Updates(r).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func RuleSets() ([]RuleSet, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var rows []RuleSet
	err = db.Order("id").Find(&rows).Error
	return rows, err
}

func ImportRules(content string) ([]string, error) {
	var doc struct {
		Payload []string `yaml:"payload"`
	}
	if err := yaml.UnmarshalWithOptions([]byte(content), &doc, yaml.Strict()); err != nil || doc.Payload == nil {
		// Plain text providers carry one classical rule per line.
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				doc.Payload = append(doc.Payload, line)
			}
		}
	}
	if err := validateRules(doc.Payload, false); err != nil {
		return nil, err
	}
	return doc.Payload, nil
}

func validateRules(rules []string, blacklist bool) error {
	if len(rules) == 0 {
		return errors.New("请至少添加一条规则")
	}
	for i, line := range rules {
		kind, value, ok := strings.Cut(line, ",")
		kind = strings.ToUpper(strings.TrimSpace(kind))
		value = strings.TrimSpace(value)
		if ok {
			rules[i] = kind + "," + value
		}
		blacklistKind := kind == "DOMAIN" || kind == "DOMAIN-SUFFIX" || kind == "DOMAIN-KEYWORD" || kind == "DOMAIN-REGEX" || kind == "DOMAIN-WILDCARD" || kind == "PROCESS-NAME" || kind == "PROCESS-PATH" || kind == "PROCESS-NAME-REGEX" || kind == "PROCESS-PATH-REGEX" || kind == "PROCESS-NAME-WILDCARD" || kind == "PROCESS-PATH-WILDCARD"
		valid := ok && value != "" && !strings.ContainsAny(value, "\r\n")
		switch kind {
		case "DOMAIN", "DOMAIN-SUFFIX":
			_, err := idna.Lookup.ToASCII(value)
			valid = valid && err == nil && strings.Trim(value, ".") != "" && !strings.HasPrefix(value, ".")
		case "DOMAIN-KEYWORD":
			valid = valid && !strings.Contains(value, ",")
		case "DOMAIN-REGEX", "PROCESS-PATH-REGEX", "PROCESS-NAME-REGEX":
			_, err := regexp2.Compile(value, regexp2.IgnoreCase)
			valid = valid && err == nil
		case "IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR":
			prefix, err := netip.ParsePrefix(strings.SplitN(value, ",", 2)[0])
			valid = valid && err == nil && prefix.IsValid()
		case "DOMAIN-WILDCARD", "GEOSITE", "GEOIP", "SRC-GEOIP", "IP-ASN", "SRC-IP-ASN", "IP-SUFFIX", "SRC-IP-SUFFIX", "SRC-PORT", "DST-PORT", "IN-PORT", "DSCP", "PROCESS-NAME", "PROCESS-PATH", "PROCESS-NAME-WILDCARD", "PROCESS-PATH-WILDCARD", "NETWORK", "UID", "IN-TYPE", "IN-USER", "IN-NAME", "REMATCH-NAME", "AND", "OR", "NOT":
			// Preserve native rule expressions and parameters for the client parser.
		default:
			valid = false
		}
		if !valid {
			return fmt.Errorf("第 %d 条规则格式或类型不支持", i+1)
		}
		if blacklist && (!blacklistKind || (!strings.HasSuffix(kind, "-REGEX") && strings.Contains(value, ","))) {
			return fmt.Errorf("第 %d 条规则不支持黑名单分类", i+1)
		}
	}
	return nil
}

func SaveRuleSet(row *RuleSet) error {
	row.Name = strings.TrimSpace(row.Name)
	if row.Name == "" || !(row.Routing || row.Blacklist) {
		return errors.New("请填写名称并选择用途")
	}
	if err := validateRules(row.Rules, row.Blacklist); err != nil {
		return err
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	row.UpdatedAt = &now
	if row.ID == 0 {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := assignPublication(tx, row); err != nil {
				return err
			}
			return tx.Create(row).Error
		})
	}
	res := db.Model(row).Select("Name", "Routing", "Blacklist", "Rules", "UpdatedAt").Updates(row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("规则集不存在")
	}
	return nil
}

func PublishedRuleFile(code, filename string) ([]byte, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var row RuleSet
	if err := db.Where("code = ? AND filename = ?", code, filename).First(&row).Error; err != nil {
		return nil, err
	}
	return yaml.Marshal(map[string][]string{"payload": row.Rules})
}

func PublishedRules(id uint) ([]byte, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var row RuleSet
	if err = db.First(&row, id).Error; err != nil {
		return nil, err
	}
	return yaml.Marshal(map[string][]string{"payload": row.Rules})
}

func DeleteRuleSet(id uint) error {
	if id == 0 {
		return errors.New("规则集 ID 无效")
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var templates []YAMLTemplate
		if err := tx.Find(&templates).Error; err != nil {
			return err
		}
		for _, template := range templates {
			for _, binding := range template.Injections.Routes {
				if binding.RuleSetID == id {
					return fmt.Errorf("规则集被模板 %q 的路由引用，请先解除引用", template.Name)
				}
			}
			for _, binding := range template.Injections.Blacklists {
				if binding.RuleSetID == id {
					return fmt.Errorf("规则集被模板 %q 的黑名单引用，请先解除引用", template.Name)
				}
			}
		}
		result := tx.Delete(&RuleSet{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("规则集不存在")
		}
		return nil
	})
}
