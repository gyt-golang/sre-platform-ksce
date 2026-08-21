// Package remediate 的错误预算策略监控器（进化功能）。
//
// 错误预算耗尽时自动冻结发布（给 Argo Rollout 打 spec.pause=true），
// 预算恢复自动解冻。体现"错误预算驱动变更策略"——预算不够就不许发，从源头防止故障扩大。
//
// 逻辑：定时（每 60s）查 Prometheus ordersvc:burn_rate1d，
//   burn_rate1d > 1（按当前速率会耗尽月预算）→ patch Rollout pause=true
//   burn_rate1d < 0.5 → 解冻 pause=false
package remediate

import (
	"context"
	"log"
	"time"

	"github.com/sre-demo/remediator/internal/k8s"
	"github.com/sre-demo/remediator/internal/prom"
)

// BudgetPolicy 错误预算策略监控器。
type BudgetPolicy struct {
	promClient  *prom.Client
	service     string
	rolloutName string
	k8sClient   *k8s.Client   // nil 时降级（非集群内不执行 pause）
	checkPeriod time.Duration
	stopCh      chan struct{}
	lastFrozen  bool // 避免反复 patch
}

// NewBudgetPolicy 创建监控器。period 默认 60s。
func NewBudgetPolicy(pc *prom.Client, service, rolloutName string, kc *k8s.Client, period time.Duration) *BudgetPolicy {
	if period == 0 {
		period = 60 * time.Second
	}
	return &BudgetPolicy{
		promClient: pc, service: service, rolloutName: rolloutName,
		k8sClient: kc, checkPeriod: period, stopCh: make(chan struct{}),
	}
}

// Start 启动定时监控（应 go Start()）。
func (b *BudgetPolicy) Start() {
	ticker := time.NewTicker(b.checkPeriod)
	defer ticker.Stop()
	log.Printf("[info] budget policy 监控启动，每 %s 查 burn_rate1d", b.checkPeriod)
	for {
		select {
		case <-ticker.C:
			b.check()
		case <-b.stopCh:
			log.Println("[info] budget policy 监控停止")
			return
		}
	}
}

// Stop 停止监控。
func (b *BudgetPolicy) Stop() { close(b.stopCh) }

// FreezeThreshold 预算耗尽阈值（burn_rate1d > 1 表示按当前速率会耗尽月预算）。
const FreezeThreshold = 1.0

// UnfreezeThreshold 解冻阈值（留余量，避免临界抖动）。
const UnfreezeThreshold = 0.5

// check 查 burn_rate1d，按阈值冻结/解冻发布。
func (b *BudgetPolicy) check() {
	if b.k8sClient == nil {
		return // 非集群内，降级跳过
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	burn1d, err := b.promClient.Query(ctx, b.service+":burn_rate1d")
	if err != nil {
		log.Printf("[warn] budget policy 查 burn_rate1d 失败: %v", err)
		return
	}
	if burn1d > FreezeThreshold && !b.lastFrozen {
		log.Printf("[warn] 错误预算耗尽风险 burn_rate1d=%.2f > %.2f，冻结 Rollout 发布", burn1d, FreezeThreshold)
		if err := b.k8sClient.PauseRollout(b.rolloutName, true); err != nil {
			log.Printf("[error] 冻结 Rollout 失败: %v", err)
		} else {
			b.lastFrozen = true
		}
	} else if burn1d < UnfreezeThreshold && b.lastFrozen {
		log.Printf("[info] 错误预算恢复 burn_rate1d=%.2f < %.2f，解冻 Rollout 发布", burn1d, UnfreezeThreshold)
		if err := b.k8sClient.PauseRollout(b.rolloutName, false); err != nil {
			log.Printf("[error] 解冻 Rollout 失败: %v", err)
		} else {
			b.lastFrozen = false
		}
	}
}
