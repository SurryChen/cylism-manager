package operations

import (
	"encoding/json"
	"fmt"

	"github.com/cylism/cylism-manager/internal/model"
	"github.com/cylism/cylism-manager/internal/security"
	application "github.com/cylism/cylism-manager/internal/service/application"
	"gorm.io/gorm"
)

// switchConnections atomically changes only explicitly selected binding
// templates. It does not overwrite the instance's global business credential,
// so multiple schemas/users on one external RDS remain independent.
func (s *Service) switchConnections(task *model.DatabaseOperation) error {
	plain, err := security.Decrypt(s.key, task.EncryptedInput)
	if err != nil {
		return fmt.Errorf("读取切换连接参数失败")
	}
	var input jobInput
	if json.Unmarshal([]byte(plain), &input) != nil {
		return fmt.Errorf("切换连接参数无效")
	}
	_, targetID, err := ParseReference(task.Target)
	if err != nil {
		return err
	}
	return s.store.DB().Transaction(func(tx *gorm.DB) error {
		var current model.DatabaseOperation
		if e := tx.First(&current, task.ID).Error; e != nil {
			return e
		}
		if current.ConnectionsApplied {
			return nil
		}
		var target model.DatabaseInstance
		if e := tx.First(&target, targetID).Error; e != nil {
			return e
		}
		for _, conn := range input.Connections {
			var binding model.DatabaseBinding
			var template model.ApplicationDeploymentTemplate
			var app model.Application
			if e := tx.First(&binding, conn.BindingID).Error; e != nil {
				return fmt.Errorf("绑定已变化，请重新创建切换任务")
			}
			if e := tx.Preload("Environment").First(&app, binding.ApplicationID).Error; e != nil {
				return e
			}
			if e := tx.First(&template, binding.TemplateID).Error; e != nil {
				return e
			}
			if template.Revision != conn.Revision {
				return fmt.Errorf("上线模板已被其他操作修改，请重新创建任务")
			}
			host, port, e := s.endpointAddress(tx, &target, app.Environment.ClusterID)
			if e != nil {
				return e
			}
			var spec application.ReleaseSpec
			if e = json.Unmarshal([]byte(template.Spec), &spec); e != nil {
				return e
			}
			if spec.Config == nil {
				spec.Config = map[string]string{}
			}
			if spec.Secrets == nil {
				spec.Secrets = map[string]string{}
			}
			secrets := map[string]string{}
			if template.EncryptedSecrets != "" {
				raw, e := security.Decrypt(s.key, template.EncryptedSecrets)
				if e != nil {
					return fmt.Errorf("读取模板 Secret 失败")
				}
				if e = json.Unmarshal([]byte(raw), &secrets); e != nil {
					return e
				}
			}
			for suffix, value := range map[string]string{"HOST": host, "PORT": fmt.Sprint(port), "NAME": conn.Database, "USER": conn.Username} {
				key := binding.Prefix + "_" + suffix
				spec.Config[key] = value
				delete(spec.Secrets, key)
				delete(secrets, key)
				spec.SecretFileKeys = remove(spec.SecretFileKeys, key)
				spec.ConfigDisabled = remove(spec.ConfigDisabled, key)
				spec.ConfigFileKeys = remove(spec.ConfigFileKeys, key)
			}
			key := binding.Prefix + "_PASSWORD"
			delete(spec.Config, key)
			spec.ConfigFileKeys = remove(spec.ConfigFileKeys, key)
			spec.Secrets[key] = ""
			secrets[key] = conn.Password
			spec.SecretsDisabled = remove(spec.SecretsDisabled, key)
			spec.SecretFileKeys = remove(spec.SecretFileKeys, key)
			raw, e := json.Marshal(secrets)
			if e != nil {
				return e
			}
			template.EncryptedSecrets, e = security.Encrypt(s.key, string(raw))
			if e != nil {
				return e
			}
			raw, e = json.Marshal(application.SanitizeReleaseSpec(spec))
			if e != nil {
				return e
			}
			template.Spec = string(raw)
			template.Revision++
			if e = tx.Save(&template).Error; e != nil {
				return e
			}
			binding.InstanceID = targetID
			binding.DatabaseName = conn.Database
			binding.Username = conn.Username
			if e = tx.Save(&binding).Error; e != nil {
				return e
			}
		}
		// The marker commits with all templates so a platform restart cannot
		// confuse another edit with this task's own successful transaction.
		return tx.Model(&current).Update("connections_applied", true).Error
	})
}
func remove(values []string, key string) []string {
	out := values[:0]
	for _, value := range values {
		if value != key {
			out = append(out, value)
		}
	}
	return out
}
