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
        kustomize build --load_restrictor none tests/ | kubectl apply -f -
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
