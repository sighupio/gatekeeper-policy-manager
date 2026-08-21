#!/usr/bin/env bats
# Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# bats false positives: SC2154 ($status/$output are set by the bats runner), SC2329 (helper
# functions are invoked indirectly by @test blocks, not called in the source).
# shellcheck disable=SC2154,SC2329

load ./helper

@test "Requirements" {
    info
    ns(){
        kubectl create ns gatekeeper-system
	# We create the CRD so the apply doesn't fail. We don't care about the servicemonitor and the rule actually
        kubectl apply -f https://raw.githubusercontent.com/sighupio/module-monitoring/v4.2.0/katalog/prometheus-operator/crds/0servicemonitorCustomResourceDefinition.yaml
        kubectl apply -f https://raw.githubusercontent.com/sighupio/module-monitoring/v4.2.0/katalog/prometheus-operator/crds/0prometheusruleCustomResourceDefinition.yaml
    }
    run ns
    [ "$status" -eq 0 ]
}

@test "Deploy" {
    info
    deploy(){
        kustomize build --load-restrictor LoadRestrictionsNone tests/ | kubectl apply -f -
        # Applied outside the kustomization on purpose: it sets `namespace: gatekeeper-system`, and
        # that transformer renames a Namespace object rather than leaving it alone.
        kubectl apply -f tests/violating-workload.yaml
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

# The UI snapshot tests (the Home dashboard and the Constraints view) render Gatekeeper's audit
# results. The audit runs a few cycles after deploy, so on a fresh cluster the views briefly show
# zero violations. Gate the whole UI-test stage on the audit here, once, instead of making every
# snapshot spec carry its own retry loop.
@test "Wait for Gatekeeper's audit to run" {
    info
    settled(){
        # Wait until the audit status the UI renders is fully populated, on two counts:
        #  - every Constraint carries a status.auditTimestamp (the audit's full scan has run, so
        #    totalViolations is final) and the total is non-zero (the e2e cluster always has some);
        #  - every Constraint's status.byPod lists all the Gatekeeper pods (audit + controllers), so
        #    the "Status by pod" block on the Constraints page has stopped growing.
        # With both settled, the Home and Constraints snapshots need no retry loop of their own.
        local total_c audited_c total_v reporters incomplete
        total_c=$(kubectl get constraints -o jsonpath='{.items[*].metadata.name}' | wc -w)
        audited_c=$(kubectl get constraints -o jsonpath='{range .items[*]}{.status.auditTimestamp}{"\n"}{end}' | grep -c .)
        total_v=$(kubectl get constraints -o jsonpath='{range .items[*]}{.status.totalViolations}{" "}{end}' | tr ' ' '\n' | awk '{s+=$1} END{print s+0}')
        reporters=$(kubectl get pods -n gatekeeper-system -l gatekeeper.sh/system=yes --field-selector=status.phase=Running -o name | wc -l)
        incomplete=$(kubectl get constraints -o jsonpath='{range .items[*]}{.status.byPod[*].id}{"\n"}{end}' | awk -v r="$reporters" 'NF < r {c++} END{print c+0}')
        echo "audited ${audited_c}/${total_c} constraints, ${total_v} violations, ${incomplete} with incomplete byPod (reporters=${reporters})"
        [ "$total_c" -gt 0 ] && [ "$audited_c" -eq "$total_c" ] && [ "$total_v" -gt 0 ] && [ "$reporters" -gt 0 ] && [ "$incomplete" -eq 0 ]
    }
    loop_it settled 30 10
    [ "$loop_it_result" -eq 0 ]
}

@test "Trigger an admission event" {
    info
    # K8sUniqueIngressHost is the one deny constraint that matches Ingress and nothing else, so a
    # duplicate host produces exactly one gatekeeper-webhook event. That is a deterministic single
    # row for the events-view snapshot; a bare Pod trips several constraints at once, in an order
    # that is not stable. The first Ingress is admitted and stays so the second is a duplicate.
    kubectl -n default apply -f - <<'ING'
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: gpm-events-a
spec:
  rules:
    - host: gpm-events.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: x
                port:
                  number: 80
ING
    # loop_it retries until the duplicate is denied: the webhook is not enforcing the instant GPM
    # reports ready, and the constraint needs gpm-events-a in its cache, which lags the apply above.
    expect_denied(){
        out=$(kubectl -n default apply -f - <<'ING' 2>&1
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: gpm-events-b
spec:
  rules:
    - host: gpm-events.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: x
                port:
                  number: 80
ING
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
    # Read the count into a variable of our own. loop_it invokes this through bats' `run`, so $output
    # and $status inside here belong to the *previous* attempt, and are empty on the first one. The
    # old last line, `[[ "$status" -eq 0 ]]`, then compared an empty string to 0 -- true in [[ ]]
    # arithmetic -- so this test passed against a cluster that had no constraints at all.
    local violations
    violations=$(kubectl get k8slivenessprobe.constraints.gatekeeper.sh liveness-probe \
      -o jsonpath='{.status.totalViolations}' 2>/dev/null)
    echo "liveness-probe reports ${violations:-no} violations"
    [ "${violations:-0}" -eq 2 ]
  }
  loop_it wait_violations 10 5
  [ "$loop_it_result" -eq 0 ]
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
