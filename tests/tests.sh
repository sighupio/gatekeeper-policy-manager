#!/usr/bin/env bats
# Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

# bats false positives: SC2154 ($status/$output are set by the bats runner), SC2329 (helper
# functions are invoked indirectly by @test blocks, not called in the source).
# shellcheck disable=SC2154,SC2329

load ./helper

# Give kubectl a cache directory of its own, so the Deploy test can drop it. kubectl caches API
# discovery on disk. When that cache was written before Gatekeeper created the constraint CRDs, the
# first `kubectl get constraints` resolves the "constraints" category against the stale list: it
# prints nothing and exits 0. That same call refreshes the cache, so the next one is right. A retry
# loop never sees this, and a single count does. Dropping the cache keeps the counts below right
# whichever kubectl call runs first.
export KUBECACHEDIR="${TMPDIR:-/tmp}/gpm-e2e-kubectl-cache"

# kapp, and not `kubectl apply`, because the fixture holds ConstraintTemplates together with the
# Constraints that need the CRDs Gatekeeper makes from them. One apply sends both, and the
# Constraints are rejected with "no matches for kind" until those CRDs exist. That is why this test
# used to apply the whole fixture in a retry loop. kapp waits for the CRDs that tests/kapp/exists.yaml
# declares, applies the Constraints after they exist, and waits for the Deployments to become
# available. That replaces the retry loop here and the two readiness tests that followed it.
@test "Deploy" {
    info
    deploy(){
        kustomize build --load-restrictor LoadRestrictionsNone tests/ |
            kapp deploy --app gpm-e2e --file - --file tests/kapp/exists.yaml \
                --yes --wait-timeout 5m
        # Applied outside the kustomization on purpose: it sets `namespace: gatekeeper-system`, and
        # that transformer renames a Namespace object rather than leaving it alone.
        kubectl apply -f tests/violating-workload.yaml
    }
    run deploy
    echo "$output"
    [ "$status" -eq 0 ]
    # kubectl may have written its discovery cache before these CRDs existed. Drop it, or the count
    # below is the one query that pays for the stale entry. See the note on KUBECACHEDIR at the top.
    rm -rf "${KUBECACHEDIR:?}"
    # kapp reports what it applied, not what survived. A zero here names the fault where it happens,
    # instead of leaving it to surface three tests later as a timeout.
    constraints=$(kubectl get constraints -o name | wc -l | tr -d ' ')
    echo "deployed ${constraints} constraints"
    [ "${constraints:-0}" -gt 0 ]
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
    # loop_it retries until the duplicate is denied: the webhook does not enforce the instant the
    # deploy returns, and the constraint needs gpm-events-a in its cache, which lags the apply above.
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
