/**
 * Copyright (c) 2017-present SIGHUP s.r.l All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import {
  EuiButtonEmpty,
  EuiHeader,
  EuiHeaderSection,
  EuiHeaderSectionItem,
  EuiHideFor,
  EuiSuperSelect
} from "fury-design-system";
import "./Style.css";
import {useContext, useEffect, useState} from "react";
import {ApplicationContext} from "../../AppContext";
import {EuiSuperSelectOption} from "fury-design-system/src/components/form/super_select/super_select_control";

function HeaderComponent() {
  const [optionsFromContexts, setOptionsFromContexts] = useState<EuiSuperSelectOption<string>[]>([]);
  const {
    context,
    setContext,
  } = useContext(ApplicationContext);

  useEffect(() => {
    const optionsFromContexts = context.k8sContexts.map(k8sContext => {
      return {
        value: k8sContext,
        inputDisplay: k8sContext,
        dropdownDisplay: k8sContext
      };
    });
    setOptionsFromContexts(optionsFromContexts);
  }, [context.k8sContexts]);

  const onChangeContext = (value: string) => {
    if (setContext) {
      setContext({
        currentK8sContext: value,
      });
    }
  };

  return (
    <div className="gpm-header">
      <EuiHideFor sizes={["xs", "s"]}>
        <EuiHeader className="gpm-header--desktop">
          <EuiHeaderSection side="left">
            <EuiHeaderSectionItem>
              <EuiButtonEmpty href="/">
                GPM
              </EuiButtonEmpty>
            </EuiHeaderSectionItem>
            <EuiHeaderSectionItem>
              <EuiButtonEmpty href="/constrainttemplates">
                Constraint Templates
              </EuiButtonEmpty>
            </EuiHeaderSectionItem>
            <EuiHeaderSectionItem>
              <EuiButtonEmpty href="/constraints">
                Constraints
              </EuiButtonEmpty>
            </EuiHeaderSectionItem>
            <EuiHeaderSectionItem>
              <EuiButtonEmpty href="/configurations">
                Configurations
              </EuiButtonEmpty>
            </EuiHeaderSectionItem>
          </EuiHeaderSection>
          { optionsFromContexts.length > 0 &&
            <EuiHeaderSection side="right">
              <EuiHeaderSectionItem>
                <EuiSuperSelect
                    options={optionsFromContexts}
                    valueOfSelected={context.currentK8sContext}
                    onChange={(value) => onChangeContext(value)}
                />
              </EuiHeaderSectionItem>
            </EuiHeaderSection>
          }
        </EuiHeader>
      </EuiHideFor>
    </div>
  )
}

export default HeaderComponent;
