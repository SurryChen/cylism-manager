package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	sdkcredentials "github.com/aliyun/alibaba-cloud-sdk-go/sdk/auth/credentials"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	cloudservice "github.com/cylism/cylism-manager/internal/service/cloud"
)

// Use the SDK's signed common RPC client, avoiding a second cloud credential
// store and the very large generated RDS package for a single read-only API.
func (p *aliyunProvider) DatabaseMetrics(ctx context.Context, region, instance string, start, end time.Time) (cloudservice.DatabaseMetrics, error) {
	if err := ctx.Err(); err != nil {
		return cloudservice.DatabaseMetrics{}, err
	}
	client, err := sdk.NewClientWithOptions(region, sdk.NewConfig().WithScheme("HTTPS").WithTimeout(15*time.Second), sdkcredentials.NewAccessKeyCredential(p.accessKeyID, p.accessKeySecret))
	if err != nil {
		return cloudservice.DatabaseMetrics{}, fmt.Errorf("初始化 RDS 监控客户端失败")
	}
	req := requests.NewCommonRequest()
	req.Scheme, req.Domain, req.Version, req.ApiName, req.Product = "HTTPS", "rds.aliyuncs.com", "2014-08-15", "DescribeDBInstancePerformance", "Rds"
	req.Method = "POST"
	req.QueryParams["RegionId"], req.QueryParams["DBInstanceId"] = region, instance
	req.QueryParams["StartTime"], req.QueryParams["EndTime"] = start.UTC().Format("2006-01-02T15:04Z"), end.UTC().Format("2006-01-02T15:04Z")
	req.QueryParams["Key"] = "MySQL_MemCpuUsage,MySQL_DetailedSpaceUsage,MySQL_Sessions,MySQL_QPSTPS"
	req.QueryParams["UseNullWhenMissingPoint"] = "true"
	response, err := client.ProcessCommonRequest(req)
	if err != nil {
		// Never return a signed request, AccessKey or provider response body.
		var coded interface{ ErrorCode() string }
		message := "读取 RDS 监控失败，请检查 rds:DescribeDBInstancePerformance 权限、地域和实例 ID"
		if v, ok := err.(interface{ ErrorCode() string }); ok {
			coded = v
		}
		if coded != nil && aliyunDiagnosticID.MatchString(coded.ErrorCode()) {
			message += "；错误码：" + coded.ErrorCode()
		}
		return cloudservice.DatabaseMetrics{}, fmt.Errorf("%s", message)
	}
	return parseRDSMetrics([]byte(response.GetHttpContentString()), instance)
}

func parseRDSMetrics(raw []byte, instance string) (cloudservice.DatabaseMetrics, error) {
	var payload struct {
		PerformanceKeys struct {
			PerformanceKey []struct {
				Key         string
				Unit        string
				ValueFormat string
				Values      struct {
					PerformanceValue []struct {
						Value string
						Date  string
					}
				}
			}
		}
	}
	if json.Unmarshal(raw, &payload) != nil {
		return cloudservice.DatabaseMetrics{}, fmt.Errorf("RDS 监控响应格式无效")
	}
	result := cloudservice.DatabaseMetrics{InstanceID: instance, CollectedAt: time.Now().UTC(), Series: []cloudservice.DatabaseMetricSeries{}}
	labels := map[string]string{"cpu_usage": "CPU 使用率", "mem_usage": "内存使用率", "ins_size": "磁盘占用", "data_size": "数据空间", "log_size": "日志空间", "tmp_size": "临时空间", "other_size": "其他空间", "total_session": "总连接数", "active_session": "活跃连接", "qps": "QPS", "tps": "TPS"}
	for _, key := range payload.PerformanceKeys.PerformanceKey {
		fields, units := strings.Split(key.ValueFormat, "&"), strings.Split(key.Unit, "&")
		for i, field := range fields {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			fieldName := strings.ToLower(field)
			// RDS returns both cpu_usage/mem_usage and cpuusage/memusage.
			label := labels[fieldName]
			if label == "" {
				for name, candidate := range labels {
					if strings.ReplaceAll(name, "_", "") == strings.ReplaceAll(fieldName, "_", "") {
						label = candidate
						break
					}
				}
			}
			if label == "" {
				label = field
			}
			unit := key.Unit
			if len(units) == len(fields) {
				unit = units[i]
			}
			// Numeric data types describe storage, not a display unit.
			switch strings.ToLower(strings.TrimSpace(unit)) {
			case "int", "integer", "float", "double":
				unit = ""
			}
			series := cloudservice.DatabaseMetricSeries{Key: key.Key + "/" + field, Label: label, Unit: unit, Values: []cloudservice.DatabaseMetricPoint{}}
			for _, sample := range key.Values.PerformanceValue {
				values := strings.Split(sample.Value, "&")
				if len(values) != len(fields) {
					continue
				}
				value, e := strconv.ParseFloat(strings.TrimSpace(values[i]), 64)
				if e != nil || math.IsNaN(value) || math.IsInf(value, 0) {
					continue
				}
				at, e := time.Parse(time.RFC3339, sample.Date)
				if e != nil {
					at, e = time.Parse("2006-01-02T15:04Z", sample.Date)
				}
				if e != nil {
					continue
				}
				series.Values = append(series.Values, cloudservice.DatabaseMetricPoint{Timestamp: at.Unix(), Value: value})
			}
			result.Series = append(result.Series, series)
		}
	}
	return result, nil
}
