#!/usr/bin/env bats
# Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# shellcheck disable=SC2154

load ./helper

@test "Requirements" {
    info
    ns(){
        kubectl create ns gatekeeper-system
	# We create the CRD so the apply doesn't fail. We don't care about the servicemonitor and the rule actually
        kubectl apply -f https://raw.githubusercontent.com/sighupio/module-monitoring/v3.5.0/katalog/prometheus-operator/crds/0servicemonitorCustomResourceDefinition.yaml
        kubectl apply -f https://raw.githubusercontent.com/sighupio/module-monitoring/v3.5.0/katalog/prometheus-operator/crds/0prometheusruleCustomResourceDefinition.yaml
    }
    run ns
    [ "$status" -eq 0 ]
}

@test "Deploy" {
    info
    deploy(){
        kustomize build --load-restrictor LoadRestrictionsNone tests/ | kubectl apply -f -
    }
    loop_it deploy 10 5
    status=${loop_it_result}
    [ "$status" -eq 0 ]
}

@test "Wait until Gatekeeper Controller is ready" {
    info
    ready(){
        kubectl -n gatekeeper-system wait --for=condition=available --timeout=120s deployment/gatekeeper-controller-manager
    }
    run ready
    [ "$status" -eq 0 ]
}

@test "Wait until GPM is ready" {
    info
    ready(){
        kubectl -n gatekeeper-system wait --for=condition=available --timeout=120s deployment/gatekeeper-policy-manager
    }
    run ready
    [ "$status" -eq 0 ]
}

@test "Trigger an admission event" {
    info
    # A bare Pod violates the deny constraints (no liveness/readiness probe, no limits). With
    # --emit-admission-events on, gatekeeper rejects it at admission and writes a gatekeeper-webhook
    # Event -- what the events view reads. default is not an excluded namespace.
    #
    # loop_it retries until the pod is denied, because the webhook is not enforcing the instant GPM
    # reports ready: the ValidatingWebhookConfiguration and the constraints sync a moment later.
    # Each attempt deletes first, so an apply that slips through before enforcement leaves nothing
    # behind. The winning, denied apply is a real request, so it emits the event; a dry-run would
    # not, because gatekeeper skips side effects on dry-run.
    expect_denied(){
        kubectl -n default delete pod gpm-events-probe --ignore-not-found >/dev/null 2>&1
        out=$(kubectl -n default apply -f - <<'POD' 2>&1
apiVersion: v1
kind: Pod
metadata:
  name: gpm-events-probe
spec:
  containers:
    - name: c
      image: registry.k8s.io/pause:3.9
POD
)
        echo "$out" | grep -q "denied the request"
    }
    loop_it expect_denied 30 5
    [ "$loop_it_result" -eq 0 ]
}

@test "Run tests" {
    info
    deploy_test(){
        kubectl -n kube-system apply -f tests/e2e-tests.yaml
    }
    run deploy_test
    [ "$status" -eq 0 ]
}

@test "Check tests result" {
    info
    test(){
        kubectl -n kube-system wait --for=condition=complete --timeout=300s job/e2e-tests
    }
    run test
    [ "$status" -eq 0 ]
}

@test "[AUDIT] check violations are present" {
  info
  wait_violations(){
    kubectl get k8slivenessprobe.constraints.gatekeeper.sh liveness-probe -o go-template="{{.status.totalViolations}}"
    echo "number of violations for liveness-probe constraint is: ${output}"
    echo "command status is: ${status}"
    [[ "$output" -eq 2 ]]
    [[ "$status" -eq 0 ]]
  }
  loop_it wait_violations 10 5
}

# Test chart installation, `helm template` is not enough to test the chart actually works.
#
# The chart uses its own namespace here. Its ServiceAccount name is fixed, so it collides with the
# one the manifests above create. The ClusterRole is cluster-scoped and needs a different name.
@test "[CHART] installs and becomes ready" {
    info
    install(){
        helm install gpmchart chart/ \
            --namespace gpmchart --create-namespace \
            --set image.repository="${LOAD_IMAGE%:*}" \
            --set image.tag="${LOAD_IMAGE##*:}" \
            --set clusterRole.name=gpmchart-crd-view \
            --wait --timeout 3m
    }
    run install
    echo "$output"
    [ "$status" -eq 0 ]
}

# `helm template` cannot show whether the ServiceAccount can read what the views need. Ask the API
# server. This check finds the missing rules.
@test "[CHART] grants every permission the views need" {
    info
    sa=system:serviceaccount:gpmchart:gatekeeper-policy-manager
    for resource in \
        k8slivenessprobe.constraints.gatekeeper.sh \
        constrainttemplates.templates.gatekeeper.sh \
        configs.config.gatekeeper.sh \
        assign.mutations.gatekeeper.sh \
        events; do
        run kubectl auth can-i list "$resource" --as="$sa"
        echo "can-i list $resource -> $output"
        [ "$status" -eq 0 ]
    done
}

@test "[CHART] uninstalls cleanly" {
    info
    run helm uninstall gpmchart --namespace gpmchart --wait
    echo "$output"
    [ "$status" -eq 0 ]
}

# Teardown gets called after each test.
# There's also teardown_file that gets called once but I could not make it work.
# Leving this for debug purposes
teardown() {
    echo
    echo " ---------| EVENTS |-------- "
    kubectl get events
    echo
    echo " ---------| PODS |-------- "
    kubectl get pods -A
    echo
    echo " ---------| PODS DESCRIPTION |-------- "
    kubectl describe pods -n gatekeeper-system
    echo
    echo " ---------| GATEKEEPER LOGS |-------- "
    kubectl logs -n gatekeeper-system --selector gatekeeper.sh/system=yes
    echo
    echo " ---------| KUBEPROXY LOGS |-------- "
    kubectl logs -n kube-system --selector k8s-app=kube-proxy
    echo
    echo " ---------| LOCAL STORAGE LOGS |-------- "
    kubectl logs -n local-path-storage --selector app=local-path-provisioner

    # Don't fail test if teardown fails for some reason
    return 0
}
