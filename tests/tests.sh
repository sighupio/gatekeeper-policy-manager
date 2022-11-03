#!/usr/bin/env bats
# Copyright (c) 2022 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# shellcheck disable=SC2154

load ./helper

@test "Requirements" {
    info
    ns(){
        kubectl create ns gatekeeper-system
        kubectl apply -f https://raw.githubusercontent.com/sighupio/fury-kubernetes-monitoring/v2.0.0/katalog/prometheus-operator/crds/0servicemonitorCustomResourceDefinition.yaml
    }
    run ns
    [ "$status" -eq 0 ]
}

@test "Deploy" {
    info
    deploy(){
        kustomize build --load_restrictor none tests/ | kubectl apply -f -
    }
    loop_it deploy 30 5
    status=${loop_it_result}
    [ "$status" -eq 0 ]
}

@test "Wait until Gatekeeper Controller is ready" {
    info
    ready(){
        kubectl -n gatekeeper-system wait --for=condition=available --timeout=1200s deployment/gatekeeper-controller-manager
    }
    run ready
    [ "$status" -eq 0 ]
}

@test "Wait until GPM is ready" {
    info
    ready(){
        kubectl -n gatekeeper-system wait --for=condition=available --timeout=1200s deployment/gatekeeper-policy-manager
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

@test "Check tests" {
    info
    test(){
        kubectl -n kube-system wait --for=condition=complete --timeout=600s job/e2e-tests
    }
    run test
    [ "$status" -eq 0 ]
}

# Teardown gets called after each test.
# There's also teardown_file that gets called once but I could not make it work.
# Leving this for debug purposes
teardown() {
    kubectl get pods -A -o wide
    kubectl get events >&3
    # Don't fail test if teardown fails for some reason
    return 0
}
