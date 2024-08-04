# Canary Operator
Kubernetes Deployment 배포 전략으로는 Blue-Green 배포 전략과 Canary 배포 전략이 있습니다. Blue-Green 배포 전략은 두 개의 동일한 환경(Blue와 Green)을 운영하여 새로운 버전을 한 환경에 배포한 후 문제가 없으면 트래픽을 그 환경으로 전환하는 방식입니다.
반면, Canary 배포 전략은 새로운 버전의 배포를 일부 사용자에게만 먼저 노출시키고, 문제가 없음을 확인한 후 점진적으로 모든 사용자에게 노출시키는 전략입니다. 이 방법은 새로운 버전이 전체 사용자에게 영향을 주기 전에 소규모 사용자 그룹에서 검증할 수 있는 기회를 제공합니다.
초기 배포 단계에서 문제가 발견되면 빠르게 수정할 수 있으며, 이를 통해 사용자 경험을 개선하고 배포 리스크를 줄일 수 있습니다.
Canary Operator는 Kubernetes 환경에서 Canary 배포 전략을 간편하게 사용할 수 있도록 도와주는 도구입니다. 이 Operator를 사용하면, 새로운 버전을 점진적으로 배포하고 모니터링하여 안정성을 확인한 후 전체 사용자에게 배포할 수 있습니다.
Kubernetes의 자동화 기능을 활용하여 Canary 배포를 효율적으로 관리할 수 있으며, 이는 시스템의 가용성과 안정성을 높이는 데 큰 도움이 됩니다. Canary Operator는 Canary 리소스를 생성하고 관리하며, 배포 과정을 자동화하여 사용자의 개입을 최소화합니다.

# Canary Operator 구성
Canary Operator를 구성하는 방법은 다음과 같습니다.
```yaml
apiVersion: canary.k8shuginn.io/v1alpha1
kind: Canary
metadata:
  name: canary-sample
  namespace: default
spec:
  oldDeployment: old-deployment
  newDeployment: new-deployment
  totalReplicas: 10
  stepReplicas: 2
  cronSchedule: "* * * * *"
  enableRollback: true
```
- oldDeployment: 이전 버전의 Deployment 이름
- newDeployment: 새로운 버전의 Deployment 이름
- totalReplicas: 전체 Replicas 수
- stepReplicas: 한 번에 배포할 Replicas 수
- cronSchedule: 배포 스케줄 (Cron 표현식 : 분 시 일 월 요일)
- enableRollback: 문제 발생 시 롤백 기능 활성화 여부를 나타냅니다. (true: 활성화, false: 비활성화)

# Canary 리소스 사용하기
Canary 리소스를 생성하였다면 바로 동작하는 것이 아닌 Pending 상태로 대기하며, Canary가 정상적으로 동작하기 위해서는 이전 버전의 oldDeployment와 새로운 버전의 newDeployment가 존재해야 합니다.
만약 oldDeployment 또는 newDeployment가 존재하지 않는다면 Canary 리소스는 Error 상태로 변경됩니다. 이를 통해 사용자는 Canary 리소스의 상태를 확인하고, 배포를 시작할 준비가 되었을 때 Canary 리소스를 실행할 수 있습니다.
Canary 리소스가 Pending 상태로 대기 중인지, 정상적으로 배포 준비가 되었는지, 혹은 Error 상태에 있는지를 확인할 수 있습니다.
이렇게 Canary 배포 전략을 사용하면 새로운 버전을 점진적으로 배포하고, 발생할 수 있는 문제를 초기에 감지하여 빠르게 대응할 수 있습니다.
이를 통해 시스템의 안정성과 가용성을 높이며, 사용자 경험을 개선할 수 있습니다. Canary Operator와 함께 사용되는 Kubernetes의 자동화 기능은 이러한 배포 과정을 더욱 효율적으로 관리할 수 있게 도와줍니다.
```bash  
이는 Canary Operator가 배포를 시작하기 전에 사용자가 확인할 수 있도록 하기 위함입니다. Canary 리소스를 확인하려면 다음 명령어를 사용합니다.
```bash
kubectl get canaries.canary.k8shuginn.io -o wide
# Result
NAME            OLDREPLICAS   NEWREPLICAS   CURRENTSTEP   STATE   MESSAGE
canary-sample   10            0             0             stop    Canary is Pending
```
- OLDREPLICAS: 이전 버전의 Replicas 수
- NEWREPLICAS: 새로운 버전의 Replicas 수
- CURRENTSTEP: 현재 배포 단계
- STATE: Canary 상태
- MESSAGE: 상태 메시지

Canary 리소스를 확인한 후 배포를 시작하려면 다음같이 apply 명령어를 사용하여 Canary를 실행합니다.
```bash
kubectl annotate canary canary-sample canary.k8shuginn.io/command=apply

kubectl get canaries.canary.k8shuginn.io -o wide
# Result
NAME            OLDREPLICAS   NEWREPLICAS   CURRENTSTEP   STATE     MESSAGE
canary-sample   8             2             1             running   Canary is Running
```
canary.k8shuginn.io/command의 종류는 다음과 같습니다.
- apply: 배포를 시작 또는 재개합니다.
- stop: 배포를 일시 중지합니다.
- rollback: 즉시 이전 버전으로 롤백을 수행합니다.
- completion: 즉시 새로운 버전으로 전환합니다.

# Canary Operator Rollback
Canary 배포 중 문제가 발생하였을 경우, Canary Operator는 자동으로 롤백을 수행합니다. 롤백은 Canary 리소스의 enableRollback 필드를 true로 설정되어 있으면, 배포 중 문제가 발생할 경우 Canary Operator가 자동으로 롤백을 수행합니다. 이 설정은 시스템의 안정성을 유지하는 데 중요한 역할을 하며, 배포 중단이나 오류 발생 시 빠르게 원래 상태로 복구할 수 있습니다. 이를 통해 지속적인 서비스 가용성을 보장할 수 있습니다.
롤백이 수행되었을 경우, oldDeployment로 롤백되며 Canary 리소스의 상태는 다음과 같이 롤백을 수행한 시간과 함께 표시됩니다.
```bash
kubectl get canaries.canary.k8shuginn.io -o wide
# Result
NAME            OLDREPLICAS   NEWREPLICAS   CURRENTSTEP   STATE   MESSAGE
canary-sample   10            0             0             stop    [2024-08-03T22:00:32+09:00] Canary is rollbacked
```

# Canary Operator Completion
Canary 배포가 완료되었을 경우, 다음과 같이 Canary 리소스의 상태가 complete로 변경됩니다. 이 상태는 Canary 배포가 완료되었음을 나타내며, 사용자는 새로운 버전의 배포가 안정적으로 완료되었음을 확인할 수 있습니다.
```bash
kubectl get canaries.canary.k8shuginn.io -o wide
# Result
NAME            OLDREPLICAS   NEWREPLICAS   CURRENTSTEP   STATE      MESSAGE
canary-sample   0             10            5             complete   Canary is complete
```