// Package k8s 提供 in-cluster Kubernetes 操作，供 remediator 执行 scale/rollout undo 动作。
//
// scale 走 client-go clientset（AppsV1 UpdateScale）；
// rollout undo 走 kubectl 子进程（remediator 镜像内置 kubectl，避免引入 argoproj clientset 重依赖）。
package k8s

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client 封装 clientset 与 namespace。
type Client struct {
	clientset *kubernetes.Clientset
	namespace string
}

// New 从 in-cluster config 创建 client。remediator 须在集群内运行。
func New(namespace string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w（remediator 须在集群内运行）", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{clientset: cs, namespace: namespace}, nil
}

// ScaleUp 把 deployment 扩容到 replicas。
func (c *Client) ScaleUp(name string, replicas int32) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	scale, err := c.clientset.AppsV1().Deployments(c.namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get scale %s: %w", name, err)
	}
	scale.Spec.Replicas = replicas
	_, err = c.clientset.AppsV1().Deployments(c.namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	if err != nil {
		return "", fmt.Errorf("update scale %s: %w", name, err)
	}
	return fmt.Sprintf("scaled deployment/%s to %d", name, replicas), nil
}

// RolloutUndo 用 kubectl 让 argo rollout abort 回退到 stable。
// remediator 容器内置 kubectl + kubeconfig（in-cluster serviceaccount token）。
func (c *Client) RolloutUndo(name string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	// argo rollouts abort <name>（需 kubectl-argo-rollouts 插件，或用 kubectl patch status.abort）。
	// 用 kubectl patch 更通用：给 Rollout 打 abort。
	cmd := exec.CommandContext(ctx, "kubectl", "-n", c.namespace,
		"patch", "rollout", name, "--type=merge", "-p", `{"status":{"abort":true}}`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("rollout undo %s: %w: %s", name, err, string(out))
	}
	return fmt.Sprintf("aborted rollout/%s, 回退到 stable", name), nil
}

// WritePostmortemDraft 阶段四：把 LLM 生成的 postmortem 草稿落成 ConfigMap，
// 供 deploy/scripts/validate-postmortem.py 校验（草稿按 template.md 结构）。
// namespace 用 sre-demo（postmortem 校验脚本扫该 ns 的 ConfigMap 或本地文件）。
func (c *Client) WritePostmortemDraft(eventID, draft string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmName := "postmortem-auto-" + eventID
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: c.namespace,
			Labels:    map[string]string{"type": "postmortem", "auto-generated": "true"},
		},
		Data: map[string]string{"postmortem.md": draft},
	}
	existing, err := c.clientset.CoreV1().ConfigMaps(c.namespace).Get(ctx, cmName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = c.clientset.CoreV1().ConfigMaps(c.namespace).Create(ctx, cm, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	existing.Data = cm.Data
	_, err = c.clientset.CoreV1().ConfigMaps(c.namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

// PauseRollout 阶段四进化功能：错误预算策略自动化，给 Argo Rollout 打 spec.pause 冻结/解冻发布。
// pause=true 冻结（预算耗尽不许发），pause=false 解冻（预算恢复）。
func (c *Client) PauseRollout(name string, pause bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pauseStr := "false"
	if pause {
		pauseStr = "true"
	}
	cmd := exec.CommandContext(ctx, "kubectl", "-n", c.namespace,
		"patch", "rollout", name, "--type=merge",
		"-p", `{"spec":{"pause":`+pauseStr+`}}`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("patch rollout pause %s=%s: %w: %s", name, pauseStr, err, string(out))
	}
	return nil
}
