#!/usr/bin/env bats
# Copyright (c) 2020 SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# shellcheck disable=SC2154

load ./helper

@test "Requirements" {
    info
    ns(){
        kubectl create ns gatekeeper-system
        kubectl apply -f https://raw.githubusercontent.com/sighupio/fury-kubernetes-monitoring/v1.10.2/katalog/prometheus-operator/crd-servicemonitor.yml
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

@test "Wait until GPM is ready" {
    info
    ready(){
        kubectl -n gatekeeper-system wait --for=condition=available --timeout=300s deployment/gatekeeper-policy-manager
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
        kubectl -n kube-system wait --for=condition=complete --timeout=30s job/e2e-tests
    }
    run test
    [ "$status" -eq 0 ]
}
