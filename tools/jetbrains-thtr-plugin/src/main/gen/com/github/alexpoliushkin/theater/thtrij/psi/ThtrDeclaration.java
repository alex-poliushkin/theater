// This is a generated file. Not intended for manual editing.
package com.github.alexpoliushkin.theater.thtrij.psi;

import java.util.List;
import org.jetbrains.annotations.*;
import com.intellij.psi.PsiElement;

public interface ThtrDeclaration extends PsiElement {

  @Nullable
  ThtrActDeclaration getActDeclaration();

  @Nullable
  ThtrAuthDeclaration getAuthDeclaration();

  @Nullable
  ThtrBackendDeclaration getBackendDeclaration();

  @Nullable
  ThtrBindStatement getBindStatement();

  @Nullable
  ThtrCallDeclaration getCallDeclaration();

  @Nullable
  ThtrCaptureAuthStatement getCaptureAuthStatement();

  @Nullable
  ThtrDependencyStatement getDependencyStatement();

  @Nullable
  ThtrDoStatement getDoStatement();

  @Nullable
  ThtrEventuallyStatement getEventuallyStatement();

  @Nullable
  ThtrExpectStatement getExpectStatement();

  @Nullable
  ThtrExportStatement getExportStatement();

  @Nullable
  ThtrHttpBlock getHttpBlock();

  @Nullable
  ThtrIdentityDeclaration getIdentityDeclaration();

  @Nullable
  ThtrLogStatement getLogStatement();

  @Nullable
  ThtrNameStatement getNameStatement();

  @Nullable
  ThtrPoolDeclaration getPoolDeclaration();

  @Nullable
  ThtrPreflightStatement getPreflightStatement();

  @Nullable
  ThtrPropStatement getPropStatement();

  @Nullable
  ThtrRecordDeclaration getRecordDeclaration();

  @Nullable
  ThtrScenarioDeclaration getScenarioDeclaration();

  @Nullable
  ThtrSessionDeclaration getSessionDeclaration();

  @Nullable
  ThtrStageDeclaration getStageDeclaration();

  @Nullable
  ThtrStateBlock getStateBlock();

  @Nullable
  ThtrTransitionStatement getTransitionStatement();

}
