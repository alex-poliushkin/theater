// This is a generated file. Not intended for manual editing.
package com.github.alexpoliushkin.theater.thtrij.psi.impl;

import java.util.List;
import org.jetbrains.annotations.*;
import com.intellij.lang.ASTNode;
import com.intellij.psi.PsiElement;
import com.intellij.psi.PsiElementVisitor;
import com.intellij.psi.util.PsiTreeUtil;
import static com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes.*;
import com.intellij.extapi.psi.ASTWrapperPsiElement;
import com.github.alexpoliushkin.theater.thtrij.psi.*;

public class ThtrDeclarationImpl extends ASTWrapperPsiElement implements ThtrDeclaration {

  public ThtrDeclarationImpl(@NotNull ASTNode node) {
    super(node);
  }

  public void accept(@NotNull ThtrVisitor visitor) {
    visitor.visitDeclaration(this);
  }

  @Override
  public void accept(@NotNull PsiElementVisitor visitor) {
    if (visitor instanceof ThtrVisitor) accept((ThtrVisitor)visitor);
    else super.accept(visitor);
  }

  @Override
  @Nullable
  public ThtrActDeclaration getActDeclaration() {
    return findChildByClass(ThtrActDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrAuthDeclaration getAuthDeclaration() {
    return findChildByClass(ThtrAuthDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrBackendDeclaration getBackendDeclaration() {
    return findChildByClass(ThtrBackendDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrCallDeclaration getCallDeclaration() {
    return findChildByClass(ThtrCallDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrCaptureAuthStatement getCaptureAuthStatement() {
    return findChildByClass(ThtrCaptureAuthStatement.class);
  }

  @Override
  @Nullable
  public ThtrDependencyStatement getDependencyStatement() {
    return findChildByClass(ThtrDependencyStatement.class);
  }

  @Override
  @Nullable
  public ThtrDoStatement getDoStatement() {
    return findChildByClass(ThtrDoStatement.class);
  }

  @Override
  @Nullable
  public ThtrEventuallyStatement getEventuallyStatement() {
    return findChildByClass(ThtrEventuallyStatement.class);
  }

  @Override
  @Nullable
  public ThtrExpectStatement getExpectStatement() {
    return findChildByClass(ThtrExpectStatement.class);
  }

  @Override
  @Nullable
  public ThtrExportStatement getExportStatement() {
    return findChildByClass(ThtrExportStatement.class);
  }

  @Override
  @Nullable
  public ThtrHttpBlock getHttpBlock() {
    return findChildByClass(ThtrHttpBlock.class);
  }

  @Override
  @Nullable
  public ThtrIdentityDeclaration getIdentityDeclaration() {
    return findChildByClass(ThtrIdentityDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrLogStatement getLogStatement() {
    return findChildByClass(ThtrLogStatement.class);
  }

  @Override
  @Nullable
  public ThtrNameStatement getNameStatement() {
    return findChildByClass(ThtrNameStatement.class);
  }

  @Override
  @Nullable
  public ThtrPoolDeclaration getPoolDeclaration() {
    return findChildByClass(ThtrPoolDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrPropStatement getPropStatement() {
    return findChildByClass(ThtrPropStatement.class);
  }

  @Override
  @Nullable
  public ThtrRecordDeclaration getRecordDeclaration() {
    return findChildByClass(ThtrRecordDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrScenarioDeclaration getScenarioDeclaration() {
    return findChildByClass(ThtrScenarioDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrSessionDeclaration getSessionDeclaration() {
    return findChildByClass(ThtrSessionDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrStageDeclaration getStageDeclaration() {
    return findChildByClass(ThtrStageDeclaration.class);
  }

  @Override
  @Nullable
  public ThtrStateBlock getStateBlock() {
    return findChildByClass(ThtrStateBlock.class);
  }

  @Override
  @Nullable
  public ThtrTransitionStatement getTransitionStatement() {
    return findChildByClass(ThtrTransitionStatement.class);
  }

}
