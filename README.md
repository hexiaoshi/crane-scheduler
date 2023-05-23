# Crane-scheduler
[![Go Report Card](https://goreportcard.com/badge/github.com/gocrane/crane-scheduler)](https://goreportcard.com/report/github.com/gocrane/crane-scheduler)
[![build-images](https://github.com/kubeservice-stack/crane-scheduler/actions/workflows/build-images.yml/badge.svg?branch=main)](https://github.com/kubeservice-stack/crane-scheduler/actions/workflows/build-images.yml)

## Overview
Crane-scheduler is a collection of scheduler plugins based on [scheduler framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/), including:
Crane scheduler Plus is a Kubernetes scheduler which can schedule pod based on actual node load. **Most important is `Independent of Prometheus`**

- [Dynamic scheduler: a load-aware scheduler plugin](doc/dynamic-scheduler.md)
## Get Started
### Install `Node-Metrics`, as node-exporter plus
Make sure your kubernetes cluster has [Node-Metrics](https://github.com/kubeservice-stack/node-metrics) installed. If not, please refer to [Install Node-Metrics](https://github.com/kubeservice-stack/node-metrics/blob/master/hack/deployment/daemonset.yaml).
### Deployment `Node-Metrics`
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  labels:
    app: node-metrics
  name: node-metrics
  namespace: crane-system
spec:
  selector:
    matchLabels:
      app: node-metrics
  template:
    metadata:
      labels:
        app: node-metrics
    spec:
      containers:
      - image: dongjiang1989/node-metrics:latest
        name: node-metrics
        args:
        - --web.listen-address=0.0.0.0:19101
        resources:
          limits:
            cpu: 102m
            memory: 180Mi
          requests:
            cpu: 102m
            memory: 180Mi
      hostNetwork: true
      hostPID: true
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/master
```

### 3. Install `Scheduler` and `Controller`
There are two options:
1) Install `Scheduler` as a `second` scheduler:
   ```bash
   git clone git@github.com:kubeservice-stack/crane-scheduler.git
   cd crane-scheduler/deploy/deployment/
   kubectl apply -f .
   ```
2) Replace native Kube-scheduler with `scheduler`:
   1) Backup `/etc/kubernetes/manifests/kube-scheduler.yaml`
   ```bash
   cp /etc/kubernetes/manifests/kube-scheduler.yaml /etc/kubernetes/
   ```
   2) Modify configfile of kube-scheduler(`scheduler-config.yaml`) to enable Dynamic scheduler plugin and configure plugin args:
   ```yaml
   apiVersion: kubescheduler.config.k8s.io/v1beta2
   kind: KubeSchedulerConfiguration
   ...
   profiles:
   - schedulerName: default-scheduler
     plugins:
       filter:
         enabled:
         - name: Dynamic
       score:
         enabled:
         - name: Dynamic
           weight: 3
     pluginConfig:
     - name: Dynamic
        args:
         policyConfigPath: /etc/kubernetes/policy.yaml
   ...
   ```
   3) Create `/etc/kubernetes/policy.yaml`, using as scheduler policy of Dynamic plugin:
   ```yaml
    apiVersion: scheduler.policy.crane.io/v1alpha1
    kind: DynamicSchedulerPolicy
    spec:
      syncPolicy:
        ##cpu usage
        - name: cpu_usage_avg_5m
          period: 3m
        - name: cpu_usage_max_avg_1h
          period: 15m
        - name: cpu_usage_max_avg_1d
          period: 3h
        ##memory usage
        - name: mem_usage_avg_5m
          period: 3m
        - name: mem_usage_max_avg_1h
          period: 15m
        - name: mem_usage_max_avg_1d
          period: 3h

      predicate:
        ##cpu usage
        - name: cpu_usage_avg_5m
          maxLimitPecent: 65
        - name: cpu_usage_max_avg_1h
          maxLimitPecent: 75
        ##memory usage
        - name: mem_usage_avg_5m
          maxLimitPecent: 65
        - name: mem_usage_max_avg_1h
          maxLimitPecent: 75

      priority:
        ##cpu usage
        - name: cpu_usage_avg_5m
          weight: 0.2
        - name: cpu_usage_max_avg_1h
          weight: 0.3
        - name: cpu_usage_max_avg_1d
          weight: 0.5
        ##memory usage
        - name: mem_usage_avg_5m
          weight: 0.2
        - name: mem_usage_max_avg_1h
          weight: 0.3
        - name: mem_usage_max_avg_1d
          weight: 0.5

      hotValue:
        - timeRange: 5m
          count: 5
        - timeRange: 1m
          count: 2
   ```
   4) Modify `kube-scheduler.yaml` and replace kube-scheduler image with Crane-scheduler：
   ```yaml
   ...
    image: docker.io/dongjiang1989/crane-scheduler:main
   ...
   ```
   1) Install [cheduler-controller](deploy/controller/deployment.yaml):
    ```bash
    kubectl apply ./deploy/controller/rbac.yaml && kubectl apply -f ./deploy/controller/deployment.yaml
    ```

### 4. Schedule Pods With scheduler

#### 4.1 Test cpu Stress Case
Test scheduler with following example:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cpu-stress
spec:
  selector:
    matchLabels:
      app: cpu-stress
  replicas: 1
  template:
    metadata:
      labels:
        app: cpu-stress
    spec:
      schedulerName: crane-scheduler
      hostNetwork: true
      tolerations:
      - key: node.kubernetes.io/network-unavailable
        operator: Exists
        effect: NoSchedule
      containers:
      - name: stress
        image: docker.io/dongjiang1989/stress:latest
        command: ["stress", "-c", "1"]
        resources:
          requests:
            memory: "1Gi"
            cpu: "1"
          limits:
            memory: "1Gi"
            cpu: "1"
```
>**Note:** Change `crane-scheduler` to `default-scheduler` if `crane-scheduler` is used as default.

There will be the following event if the test pod is successfully scheduled:
```bash
Events:
  Type    Reason     Age   From             Message
  ----    ------     ----  ----             -------
  Normal  Scheduled  91s   crane-scheduler  Successfully assigned default/cpu-stress-5c64f4d6fb-wnmsj to kcs-dongjiang-s-xtl6v
  Normal  Pulling    91s   kubelet          Pulling image "docker.io/dongjiang1989/stress:latest"
  Normal  Pulled     5s    kubelet          Successfully pulled image "docker.io/dongjiang1989/stress:latest" in 1m26.001017318s
  Normal  Created    5s    kubelet          Created container stress
  Normal  Started    5s    kubelet          Started container stress
```
#### 4.2 Test Nginx `rolling update` Case

```yaml
apiVersion: apps/v1                                                                                                                                         
kind: Deployment
metadata:
  name: nginxapp
  labels:
    app: nginxapp
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      schedulerName: crane-scheduler
      containers:
      - name: nginx
        image: nginx:1.11.9-alpine
        ports:
        - containerPort: 80
```
>**Note:** Change `crane-scheduler` to `default-scheduler` if `crane-scheduler` is used as default.

```bash
$ kubectl get deployment
NAME             READY   UP-TO-DATE   AVAILABLE   AGE
nginxapp         3/3     3            3           1d
$ kubectl scale --replicas=30 deployment/nginxapp 
deployment.apps/nginxapp scaled
```

There will be the following event if the test pod is successfully scheduled:
```bash
Conditions:
  Type           Status  Reason
  ----           ------  ------
  Progressing    True    NewReplicaSetAvailable
  Available      True    MinimumReplicasAvailable
OldReplicaSets:  <none>
NewReplicaSet:   nginxapp-57bdf45cbf (30/30 replicas created)
Events:
  Type    Reason             Age                From                   Message
  ----    ------             ----               ----                   -------
  Normal  ScalingReplicaSet  56m                deployment-controller  Scaled up replica set nginxapp-57bdf45cbf to 13
  Normal  ScalingReplicaSet  56m                deployment-controller  Scaled down replica set nginxapp-664b9459f7 to 39
  Normal  ScalingReplicaSet  56m                deployment-controller  Scaled up replica set nginxapp-57bdf45cbf to 26
  Normal  ScalingReplicaSet  51m                deployment-controller  Scaled down replica set nginxapp-664b9459f7 to 3
  Normal  ScalingReplicaSet  51m                deployment-controller  Scaled down replica set nginxapp-57bdf45cbf to 2
  Normal  ScalingReplicaSet  47m                deployment-controller  Scaled down replica set nginxapp-664b9459f7 to 1
  Normal  ScalingReplicaSet  46m (x4 over 47m)  deployment-controller  (combined from similar events): Scaled up replica set nginxapp-57bdf45cbf to 40
  Normal  ScalingReplicaSet  2m52s              deployment-controller  Scaled up replica set nginxapp-57bdf45cbf to 43
  Normal  ScalingReplicaSet  94s                deployment-controller  Scaled down replica set nginxapp-57bdf45cbf to 3
  Normal  ScalingReplicaSet  37s                deployment-controller  Scaled up replica set nginxapp-57bdf45cbf to 30
```
## Compatibility Matrix

```bash
 KUBE_EDITOR="sed -i 's/v1beta2/v1beta1/g'" kubectl edit cm scheduler-config -n crane-system && kubectl edit deploy crane-scheduler -n crane-system
```
