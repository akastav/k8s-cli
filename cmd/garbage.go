package cmd

import (
	"context"
	"fmt"
	"k8s-cli/pkg/k8s"
	"os"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var garbageOutputFile string
var garbageNamespace string

// Структура для хранения информации о секрете
type SecretUsage struct {
	Name      string
	Namespace string
	UsedBy    []string
	IsUsed    bool
	Age       string
	Type      string
}

// Структура для хранения информации о ConfigMap
type ConfigMapUsage struct {
	Name      string
	Namespace string
	UsedBy    []string
	IsUsed    bool
	Age       string
}

// Структура для хранения информации о PVC
type PVCUsage struct {
	Name      string
	Namespace string
	UsedBy    []string
	IsUsed    bool
	Age       string
	Size      string
}

// Структура для хранения информации о Service
type ServiceUsage struct {
	Name      string
	Namespace string
	UsedBy    []string
	IsUsed    bool
	Age       string
	Type      string
}

// Проверка использования секретов
func checkSecretUsage(clientset *kubernetes.Clientset) ([]SecretUsage, error) {
	ctx := context.TODO()
	var results []SecretUsage

	// Получаем все секреты
	secrets, err := clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения секретов: %v", err)
	}

	// Получаем все Pod'ы
	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения подов: %v", err)
	}

	// Получаем все Deployment'ы
	deployments, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения deployment'ов: %v", err)
	}

	// Получаем все StatefulSet'ы
	statefulsets, err := clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения statefulset'ов: %v", err)
	}

	// Получаем все DaemonSet'ы
	daemonsets, err := clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения daemonset'ов: %v", err)
	}

	// Получаем все Jobs
	jobs, err := clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения jobs: %v", err)
	}

	// Получаем все CronJobs
	cronjobs, err := clientset.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения cronjobs: %v", err)
	}

	// Получаем все Ingress
	ingresses, err := clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения ingress'ов: %v", err)
	}

	// Получаем все ServiceAccount'ы
	serviceaccounts, err := clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения serviceaccount'ов: %v", err)
	}

	// Собираем все ссылки на секреты
	secretReferences := make(map[string]map[string][]string)

	// 1. Проверяем Pod'ы (volumes, env, envFrom, imagePullSecrets)
	for _, pod := range pods.Items {
		if garbageNamespace != "" && pod.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(pod.Namespace) {
			continue
		}

		// volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				addSecretRef(secretReferences, pod.Namespace, vol.Secret.SecretName, fmt.Sprintf("Pod/%s (volume)", pod.Name))
			}
		}

		// containers
		for _, container := range pod.Spec.Containers {
			// env с secretKeyRef
			for _, env := range container.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					addSecretRef(secretReferences, pod.Namespace, env.ValueFrom.SecretKeyRef.Name, fmt.Sprintf("Pod/%s (env)", pod.Name))
				}
			}
			// envFrom
			for _, envFrom := range container.EnvFrom {
				if envFrom.SecretRef != nil {
					addSecretRef(secretReferences, pod.Namespace, envFrom.SecretRef.Name, fmt.Sprintf("Pod/%s (envFrom)", pod.Name))
				}
			}
		}

		// init containers
		for _, container := range pod.Spec.InitContainers {
			for _, env := range container.Env {
				if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					addSecretRef(secretReferences, pod.Namespace, env.ValueFrom.SecretKeyRef.Name, fmt.Sprintf("Pod/%s (init-env)", pod.Name))
				}
			}
			for _, envFrom := range container.EnvFrom {
				if envFrom.SecretRef != nil {
					addSecretRef(secretReferences, pod.Namespace, envFrom.SecretRef.Name, fmt.Sprintf("Pod/%s (init-envFrom)", pod.Name))
				}
			}
		}

		// imagePullSecrets
		for _, ips := range pod.Spec.ImagePullSecrets {
			addSecretRef(secretReferences, pod.Namespace, ips.Name, fmt.Sprintf("Pod/%s (imagePullSecrets)", pod.Name))
		}
	}

	// 2-6. Проверяем Deployment, StatefulSet, DaemonSet, Jobs, CronJobs
	checkWorkloadSecrets(clientset, deployments, statefulsets, daemonsets, jobs, cronjobs, secretReferences)

	// 7. Проверяем Ingress (TLS secrets)
	for _, ing := range ingresses.Items {
		if garbageNamespace != "" && ing.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(ing.Namespace) {
			continue
		}

		for _, tls := range ing.Spec.TLS {
			if tls.SecretName != "" {
				addSecretRef(secretReferences, ing.Namespace, tls.SecretName, fmt.Sprintf("Ingress/%s (tls)", ing.Name))
			}
		}
	}

	// 8. Проверяем ServiceAccount'ы (imagePullSecrets)
	for _, sa := range serviceaccounts.Items {
		if garbageNamespace != "" && sa.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(sa.Namespace) {
			continue
		}

		for _, ips := range sa.ImagePullSecrets {
			addSecretRef(secretReferences, sa.Namespace, ips.Name, fmt.Sprintf("ServiceAccount/%s (imagePullSecrets)", sa.Name))
		}
	}

	// Формируем результаты
	for _, secret := range secrets.Items {
		if garbageNamespace != "" && secret.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(secret.Namespace) {
			continue
		}
		if secret.Type == v1.SecretTypeServiceAccountToken {
			continue
		}
		if isSystemSecret(secret.Name) {
			continue
		}

		usage := SecretUsage{
			Name:      secret.Name,
			Namespace: secret.Namespace,
			Type:      string(secret.Type),
			Age:       formatAge(secret.CreationTimestamp.Time),
			IsUsed:    false,
		}

		if nsRefs, ok := secretReferences[secret.Namespace]; ok {
			if refs, ok := nsRefs[secret.Name]; ok {
				usage.IsUsed = true
				usage.UsedBy = refs
			}
		}

		results = append(results, usage)
	}

	return results, nil
}

// Проверка секретов в workload ресурсах
func checkWorkloadSecrets(clientset *kubernetes.Clientset,
	deployments *appsv1.DeploymentList,
	statefulsets *appsv1.StatefulSetList,
	daemonsets *appsv1.DaemonSetList,
	jobs *batchv1.JobList,
	cronjobs *batchv1.CronJobList,
	secretReferences map[string]map[string][]string) {

	// Deployments
	for _, dep := range deployments.Items {
		if garbageNamespace != "" && dep.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		checkPodSpecSecrets(&dep.Spec.Template.Spec, dep.Namespace, fmt.Sprintf("Deployment/%s", dep.Name), secretReferences)
	}

	// StatefulSets
	for _, sts := range statefulsets.Items {
		if garbageNamespace != "" && sts.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(sts.Namespace) {
			continue
		}
		checkPodSpecSecrets(&sts.Spec.Template.Spec, sts.Namespace, fmt.Sprintf("StatefulSet/%s", sts.Name), secretReferences)
	}

	// DaemonSets
	for _, ds := range daemonsets.Items {
		if garbageNamespace != "" && ds.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(ds.Namespace) {
			continue
		}
		checkPodSpecSecrets(&ds.Spec.Template.Spec, ds.Namespace, fmt.Sprintf("DaemonSet/%s", ds.Name), secretReferences)
	}

	// Jobs
	for _, job := range jobs.Items {
		if garbageNamespace != "" && job.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(job.Namespace) {
			continue
		}
		checkPodSpecSecrets(&job.Spec.Template.Spec, job.Namespace, fmt.Sprintf("Job/%s", job.Name), secretReferences)
	}

	// CronJobs
	for _, cj := range cronjobs.Items {
		if garbageNamespace != "" && cj.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(cj.Namespace) {
			continue
		}
		checkPodSpecSecrets(&cj.Spec.JobTemplate.Spec.Template.Spec, cj.Namespace, fmt.Sprintf("CronJob/%s", cj.Name), secretReferences)
	}
}

// Проверка использования ConfigMap
func checkConfigMapUsage(clientset *kubernetes.Clientset) ([]ConfigMapUsage, error) {
	ctx := context.TODO()
	var results []ConfigMapUsage

	configmaps, err := clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения configmaps: %v", err)
	}

	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения подов: %v", err)
	}

	deployments, err := clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения deployment'ов: %v", err)
	}

	statefulsets, err := clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения statefulset'ов: %v", err)
	}

	daemonsets, err := clientset.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения daemonset'ов: %v", err)
	}

	jobs, err := clientset.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения jobs: %v", err)
	}

	cronjobs, err := clientset.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения cronjobs: %v", err)
	}

	// Собираем ссылки на ConfigMap
	cmReferences := make(map[string]map[string][]string)

	// Pod'ы
	for _, pod := range pods.Items {
		if garbageNamespace != "" && pod.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(pod.Namespace) {
			continue
		}

		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				addCMRef(cmReferences, pod.Namespace, vol.ConfigMap.Name, fmt.Sprintf("Pod/%s (volume)", pod.Name))
			}
		}

		for _, container := range pod.Spec.Containers {
			for _, envFrom := range container.EnvFrom {
				if envFrom.ConfigMapRef != nil {
					addCMRef(cmReferences, pod.Namespace, envFrom.ConfigMapRef.Name, fmt.Sprintf("Pod/%s (envFrom)", pod.Name))
				}
			}
		}
	}

	// Workload ресурсы
	checkWorkloadConfigMaps(deployments, statefulsets, daemonsets, jobs, cronjobs, cmReferences)

	// Формируем результаты
	for _, cm := range configmaps.Items {
		if garbageNamespace != "" && cm.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		if isSystemConfigMap(cm.Name) {
			continue
		}

		usage := ConfigMapUsage{
			Name:      cm.Name,
			Namespace: cm.Namespace,
			Age:       formatAge(cm.CreationTimestamp.Time),
			IsUsed:    false,
		}

		if nsRefs, ok := cmReferences[cm.Namespace]; ok {
			if refs, ok := nsRefs[cm.Name]; ok {
				usage.IsUsed = true
				usage.UsedBy = refs
			}
		}

		results = append(results, usage)
	}

	return results, nil
}

// Проверка ConfigMap в workload ресурсах
func checkWorkloadConfigMaps(
	deployments *appsv1.DeploymentList,
	statefulsets *appsv1.StatefulSetList,
	daemonsets *appsv1.DaemonSetList,
	jobs *batchv1.JobList,
	cronjobs *batchv1.CronJobList,
	cmReferences map[string]map[string][]string) {

	for _, dep := range deployments.Items {
		if garbageNamespace != "" && dep.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		checkPodSpecConfigMaps(&dep.Spec.Template.Spec, dep.Namespace, fmt.Sprintf("Deployment/%s", dep.Name), cmReferences)
	}

	for _, sts := range statefulsets.Items {
		if garbageNamespace != "" && sts.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(sts.Namespace) {
			continue
		}
		checkPodSpecConfigMaps(&sts.Spec.Template.Spec, sts.Namespace, fmt.Sprintf("StatefulSet/%s", sts.Name), cmReferences)
	}

	for _, ds := range daemonsets.Items {
		if garbageNamespace != "" && ds.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(ds.Namespace) {
			continue
		}
		checkPodSpecConfigMaps(&ds.Spec.Template.Spec, ds.Namespace, fmt.Sprintf("DaemonSet/%s", ds.Name), cmReferences)
	}

	for _, job := range jobs.Items {
		if garbageNamespace != "" && job.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(job.Namespace) {
			continue
		}
		checkPodSpecConfigMaps(&job.Spec.Template.Spec, job.Namespace, fmt.Sprintf("Job/%s", job.Name), cmReferences)
	}

	for _, cj := range cronjobs.Items {
		if garbageNamespace != "" && cj.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(cj.Namespace) {
			continue
		}
		checkPodSpecConfigMaps(&cj.Spec.JobTemplate.Spec.Template.Spec, cj.Namespace, fmt.Sprintf("CronJob/%s", cj.Name), cmReferences)
	}
}

// Проверка использования PVC
func checkPVCUsage(clientset *kubernetes.Clientset) ([]PVCUsage, error) {
	ctx := context.TODO()
	var results []PVCUsage

	pvcs, err := clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения PVC: %v", err)
	}

	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения подов: %v", err)
	}

	// Собираем ссылки на PVC
	pvcReferences := make(map[string]map[string][]string)

	for _, pod := range pods.Items {
		if garbageNamespace != "" && pod.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(pod.Namespace) {
			continue
		}

		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				addPVCRef(pvcReferences, pod.Namespace, vol.PersistentVolumeClaim.ClaimName, fmt.Sprintf("Pod/%s", pod.Name))
			}
		}
	}

	// Формируем результаты
	for _, pvc := range pvcs.Items {
		if garbageNamespace != "" && pvc.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(pvc.Namespace) {
			continue
		}

		usage := PVCUsage{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Age:       formatAge(pvc.CreationTimestamp.Time),
			Size:      pvc.Spec.Resources.Requests.Storage().String(),
			IsUsed:    false,
		}

		if nsRefs, ok := pvcReferences[pvc.Namespace]; ok {
			if refs, ok := nsRefs[pvc.Name]; ok {
				usage.IsUsed = true
				usage.UsedBy = refs
			}
		}

		results = append(results, usage)
	}

	return results, nil
}

// Проверка использования Service
func checkServiceUsage(clientset *kubernetes.Clientset) ([]ServiceUsage, error) {
	ctx := context.TODO()
	var results []ServiceUsage

	services, err := clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения сервисов: %v", err)
	}

	pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения подов: %v", err)
	}

	ingresses, err := clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("ошибка получения ingress'ов: %v", err)
	}

	// Собираем ссылки на сервисы
	svcReferences := make(map[string]map[string][]string)

	// Ingress -> Service
	for _, ing := range ingresses.Items {
		if garbageNamespace != "" && ing.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(ing.Namespace) {
			continue
		}

		for _, rule := range ing.Spec.Rules {
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service != nil {
						addSvcRef(svcReferences, ing.Namespace, path.Backend.Service.Name, fmt.Sprintf("Ingress/%s", ing.Name))
					}
				}
			}
		}
	}

	// Формируем результаты
	for _, svc := range services.Items {
		if garbageNamespace != "" && svc.Namespace != garbageNamespace {
			continue
		}
		if isSystemNamespace(svc.Namespace) {
			continue
		}

		// Пропускаем сервисы без селектора (внешние)
		if len(svc.Spec.Selector) == 0 && svc.Spec.Type != v1.ServiceTypeLoadBalancer && svc.Spec.Type != v1.ServiceTypeNodePort {
			continue
		}

		usage := ServiceUsage{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			Age:       formatAge(svc.CreationTimestamp.Time),
			IsUsed:    false,
		}

		// Проверяем, есть ли поды с matching labels
		for _, pod := range pods.Items {
			if pod.Namespace == svc.Namespace && matchesSelector(pod.Labels, svc.Spec.Selector) {
				usage.IsUsed = true
				usage.UsedBy = append(usage.UsedBy, fmt.Sprintf("Pod/%s", pod.Name))
			}
		}

		// Проверяем Ingress ссылки
		if nsRefs, ok := svcReferences[svc.Namespace]; ok {
			if refs, ok := nsRefs[svc.Name]; ok {
				usage.IsUsed = true
				usage.UsedBy = append(usage.UsedBy, refs...)
			}
		}

		results = append(results, usage)
	}

	return results, nil
}

// Вспомогательные функции
func addSecretRef(refs map[string]map[string][]string, namespace, secretName, resource string) {
	if refs[namespace] == nil {
		refs[namespace] = make(map[string][]string)
	}

	for _, r := range refs[namespace][secretName] {
		if r == resource {
			return
		}
	}

	refs[namespace][secretName] = append(refs[namespace][secretName], resource)
}

func addCMRef(refs map[string]map[string][]string, namespace, cmName, resource string) {
	if refs[namespace] == nil {
		refs[namespace] = make(map[string][]string)
	}

	for _, r := range refs[namespace][cmName] {
		if r == resource {
			return
		}
	}

	refs[namespace][cmName] = append(refs[namespace][cmName], resource)
}

func addPVCRef(refs map[string]map[string][]string, namespace, pvcName, resource string) {
	if refs[namespace] == nil {
		refs[namespace] = make(map[string][]string)
	}

	for _, r := range refs[namespace][pvcName] {
		if r == resource {
			return
		}
	}

	refs[namespace][pvcName] = append(refs[namespace][pvcName], resource)
}

func addSvcRef(refs map[string]map[string][]string, namespace, svcName, resource string) {
	if refs[namespace] == nil {
		refs[namespace] = make(map[string][]string)
	}

	for _, r := range refs[namespace][svcName] {
		if r == resource {
			return
		}
	}

	refs[namespace][svcName] = append(refs[namespace][svcName], resource)
}

func checkPodSpecSecrets(podSpec *v1.PodSpec, namespace, resourceName string, refs map[string]map[string][]string) {
	for _, vol := range podSpec.Volumes {
		if vol.Secret != nil {
			addSecretRef(refs, namespace, vol.Secret.SecretName, fmt.Sprintf("%s (volume)", resourceName))
		}
	}

	for _, container := range podSpec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				addSecretRef(refs, namespace, env.ValueFrom.SecretKeyRef.Name, fmt.Sprintf("%s (env)", resourceName))
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				addSecretRef(refs, namespace, envFrom.SecretRef.Name, fmt.Sprintf("%s (envFrom)", resourceName))
			}
		}
	}

	for _, container := range podSpec.InitContainers {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				addSecretRef(refs, namespace, env.ValueFrom.SecretKeyRef.Name, fmt.Sprintf("%s (init-env)", resourceName))
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				addSecretRef(refs, namespace, envFrom.SecretRef.Name, fmt.Sprintf("%s (init-envFrom)", resourceName))
			}
		}
	}

	for _, ips := range podSpec.ImagePullSecrets {
		addSecretRef(refs, namespace, ips.Name, fmt.Sprintf("%s (imagePullSecrets)", resourceName))
	}
}

func checkPodSpecConfigMaps(podSpec *v1.PodSpec, namespace, resourceName string, refs map[string]map[string][]string) {
	for _, vol := range podSpec.Volumes {
		if vol.ConfigMap != nil {
			addCMRef(refs, namespace, vol.ConfigMap.Name, fmt.Sprintf("%s (volume)", resourceName))
		}
	}

	for _, container := range podSpec.Containers {
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				addCMRef(refs, namespace, envFrom.ConfigMapRef.Name, fmt.Sprintf("%s (envFrom)", resourceName))
			}
		}
	}
}

func isSystemNamespace(ns string) bool {
	systemNamespaces := []string{
		"kube-system",
		"kube-public",
		"kube-node-lease",
	}

	for _, sys := range systemNamespaces {
		if ns == sys {
			return true
		}
	}

	if strings.HasPrefix(ns, "kube-") {
		return true
	}

	return false
}

func isSystemSecret(name string) bool {
	patterns := []string{
		"default-token",
		"kube-",
		"sh.helm.",
		"volumesnapshot-",
	}

	for _, pattern := range patterns {
		if strings.HasPrefix(name, pattern) {
			return true
		}
	}

	return false
}

func isSystemConfigMap(name string) bool {
	patterns := []string{
		"kube-",
		"extension-apiserver-",
		"root-ca-",
	}

	for _, pattern := range patterns {
		if strings.HasPrefix(name, pattern) {
			return true
		}
	}

	return false
}

func formatAge(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	} else if duration < 30*24*time.Hour {
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	} else {
		return fmt.Sprintf("%dM", int(duration.Hours()/(24*30)))
	}
}

func matchesSelector(labels map[string]string, selector map[string]string) bool {
	if len(selector) == 0 {
		return false
	}

	for key, value := range selector {
		if labels[key] != value {
			return false
		}
	}

	return true
}

var garbageCmd = &cobra.Command{
	Use:   "garbage",
	Short: "Найти неиспользуемые ресурсы",
	Long:  `Поиск кандидатов на удаление: неиспользуемые секреты, configmaps, PVC, и сервисы.`,
	Run: func(cmd *cobra.Command, args []string) {
		clientset, err := k8s.GetClientset()
		if err != nil {
			fmt.Printf("Ошибка подключения: %v\n", err)
			return
		}

		fmt.Println("🔍 Поиск неиспользуемых ресурсов...")
		fmt.Println(strings.Repeat("=", 80))

		// Проверка секретов
		fmt.Println("\n📦 СЕКРЕТЫ")
		fmt.Println(strings.Repeat("-", 80))

		secretUsage, err := checkSecretUsage(clientset)
		if err != nil {
			fmt.Printf("Ошибка проверки секретов: %v\n", err)
		} else {
			printUnusedSecrets(secretUsage)
		}

		// Проверка ConfigMap
		fmt.Println("\n📄 CONFIGMAPS")
		fmt.Println(strings.Repeat("-", 80))

		cmUsage, err := checkConfigMapUsage(clientset)
		if err != nil {
			fmt.Printf("Ошибка проверки configmaps: %v\n", err)
		} else {
			printUnusedConfigMaps(cmUsage)
		}

		// Проверка PVC
		fmt.Println("\n💾 PVC")
		fmt.Println(strings.Repeat("-", 80))

		pvcUsage, err := checkPVCUsage(clientset)
		if err != nil {
			fmt.Printf("Ошибка проверки PVC: %v\n", err)
		} else {
			printUnusedPVCs(pvcUsage)
		}

		// Проверка Service
		fmt.Println("\n🌐 SERVICES")
		fmt.Println(strings.Repeat("-", 80))

		svcUsage, err := checkServiceUsage(clientset)
		if err != nil {
			fmt.Printf("Ошибка проверки сервисов: %v\n", err)
		} else {
			printUnusedServices(svcUsage)
		}

		// Вывод в файл
		if garbageOutputFile != "" {
			err := writeGarbageReport(garbageOutputFile, secretUsage, cmUsage, pvcUsage, svcUsage)
			if err != nil {
				fmt.Printf("Ошибка записи отчета: %v\n", err)
			} else {
				fmt.Printf("\n📄 Отчет сохранен в: %s\n", garbageOutputFile)
			}
		}
	},
}

func printUnusedSecrets(secrets []SecretUsage) {
	var unused []SecretUsage
	for _, s := range secrets {
		if !s.IsUsed {
			unused = append(unused, s)
		}
	}

	if len(unused) == 0 {
		fmt.Println("✅ Все секреты используются")
		return
	}

	fmt.Printf("⚠️  Найдено %d неиспользуемых секретов:\n\n", len(unused))

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"NAMESPACE", "NAME", "TYPE", "AGE"})

	for _, s := range unused {
		table.Append([]string{s.Namespace, s.Name, s.Type, s.Age})
	}
	table.Render()
}

func printUnusedConfigMaps(cms []ConfigMapUsage) {
	var unused []ConfigMapUsage
	for _, cm := range cms {
		if !cm.IsUsed {
			unused = append(unused, cm)
		}
	}

	if len(unused) == 0 {
		fmt.Println("✅ Все ConfigMap используются")
		return
	}

	fmt.Printf("⚠️  Найдено %d неиспользуемых ConfigMap:\n\n", len(unused))

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"NAMESPACE", "NAME", "AGE"})

	for _, cm := range unused {
		table.Append([]string{cm.Namespace, cm.Name, cm.Age})
	}
	table.Render()
}

func printUnusedPVCs(pvcs []PVCUsage) {
	var unused []PVCUsage
	for _, pvc := range pvcs {
		if !pvc.IsUsed {
			unused = append(unused, pvc)
		}
	}

	if len(unused) == 0 {
		fmt.Println("✅ Все PVC используются")
		return
	}

	fmt.Printf("⚠️  Найдено %d неиспользуемых PVC:\n\n", len(unused))

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"NAMESPACE", "NAME", "SIZE", "AGE"})

	for _, pvc := range unused {
		table.Append([]string{pvc.Namespace, pvc.Name, pvc.Size, pvc.Age})
	}
	table.Render()
}

func printUnusedServices(svcs []ServiceUsage) {
	var unused []ServiceUsage
	for _, svc := range svcs {
		if !svc.IsUsed {
			unused = append(unused, svc)
		}
	}

	if len(unused) == 0 {
		fmt.Println("✅ Все сервисы используются")
		return
	}

	fmt.Printf("⚠️  Найдено %d неиспользуемых сервисов:\n\n", len(unused))

	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"NAMESPACE", "NAME", "TYPE", "AGE"})

	for _, svc := range unused {
		table.Append([]string{svc.Namespace, svc.Name, svc.Type, svc.Age})
	}
	table.Render()
}

func writeGarbageReport(filename string, secrets []SecretUsage, cms []ConfigMapUsage, pvcs []PVCUsage, svcs []ServiceUsage) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	fmt.Fprintf(file, "# Garbage Collection Report\n")
	fmt.Fprintf(file, "# Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	// Secrets
	fmt.Fprintf(file, "## Неиспользуемые секреты\n\n")
	fmt.Fprintf(file, "| Namespace | Name | Type | Age |\n")
	fmt.Fprintf(file, "|-----------|------|------|-----|\n")

	count := 0
	for _, s := range secrets {
		if !s.IsUsed {
			fmt.Fprintf(file, "| %s | %s | %s | %s |\n", s.Namespace, s.Name, s.Type, s.Age)
			count++
		}
	}
	if count == 0 {
		fmt.Fprintf(file, "*Все секреты используются*\n")
	}

	// ConfigMaps
	fmt.Fprintf(file, "\n## Неиспользуемые ConfigMap\n\n")
	fmt.Fprintf(file, "| Namespace | Name | Age |\n")
	fmt.Fprintf(file, "|-----------|------|-----|\n")

	count = 0
	for _, cm := range cms {
		if !cm.IsUsed {
			fmt.Fprintf(file, "| %s | %s | %s |\n", cm.Namespace, cm.Name, cm.Age)
			count++
		}
	}
	if count == 0 {
		fmt.Fprintf(file, "*Все ConfigMap используются*\n")
	}

	// PVCs
	fmt.Fprintf(file, "\n## Неиспользуемые PVC\n\n")
	fmt.Fprintf(file, "| Namespace | Name | Size | Age |\n")
	fmt.Fprintf(file, "|-----------|------|------|-----|\n")

	count = 0
	for _, pvc := range pvcs {
		if !pvc.IsUsed {
			fmt.Fprintf(file, "| %s | %s | %s | %s |\n", pvc.Namespace, pvc.Name, pvc.Size, pvc.Age)
			count++
		}
	}
	if count == 0 {
		fmt.Fprintf(file, "*Все PVC используются*\n")
	}

	// Services
	fmt.Fprintf(file, "\n## Неиспользуемые сервисы\n\n")
	fmt.Fprintf(file, "| Namespace | Name | Type | Age |\n")
	fmt.Fprintf(file, "|-----------|------|------|-----|\n")

	count = 0
	for _, svc := range svcs {
		if !svc.IsUsed {
			fmt.Fprintf(file, "| %s | %s | %s | %s |\n", svc.Namespace, svc.Name, svc.Type, svc.Age)
			count++
		}
	}
	if count == 0 {
		fmt.Fprintf(file, "*Все сервисы используются*\n")
	}

	return nil
}

func init() {
	garbageCmd.Flags().StringVarP(&garbageOutputFile, "output", "o", "", "Файл для сохранения отчета")
	garbageCmd.Flags().StringVarP(&garbageNamespace, "namespace", "n", "", "Проверить только указанный namespace")
	rootCmd.AddCommand(garbageCmd)
}
