// This is a generated file. Not intended for manual editing.
package com.github.alexpoliushkin.theater.thtrij.psi.impl;

import java.util.List;
import org.jetbrains.annotations.*;
import com.intellij.lang.ASTNode;
import com.intellij.psi.PsiElement;
import com.intellij.psi.PsiElementVisitor;
import com.intellij.psi.util.PsiTreeUtil;
import static com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes.*;
import com.github.alexpoliushkin.theater.thtrij.psi.*;

public class ThtrPropStatementImpl extends ThtrNamedElementImpl implements ThtrPropStatement {

  public ThtrPropStatementImpl(ASTNode node) {
    super(node);
  }

  public void accept(@NotNull ThtrVisitor visitor) {
    visitor.visitPropStatement(this);
  }

  @Override
  public void accept(@NotNull PsiElementVisitor visitor) {
    if (visitor instanceof ThtrVisitor) accept((ThtrVisitor)visitor);
    else super.accept(visitor);
  }

  @Override
  @NotNull
  public List<ThtrDescriptorRef> getDescriptorRefList() {
    return PsiTreeUtil.getChildrenOfTypeAsList(this, ThtrDescriptorRef.class);
  }

  @Override
  @NotNull
  public List<ThtrGeneratorCall> getGeneratorCallList() {
    return PsiTreeUtil.getChildrenOfTypeAsList(this, ThtrGeneratorCall.class);
  }

  @Override
  @NotNull
  public List<ThtrListValue> getListValueList() {
    return PsiTreeUtil.getChildrenOfTypeAsList(this, ThtrListValue.class);
  }

  @Override
  @NotNull
  public List<ThtrObjectValue> getObjectValueList() {
    return PsiTreeUtil.getChildrenOfTypeAsList(this, ThtrObjectValue.class);
  }

  @Override
  @NotNull
  public List<ThtrSelectorCall> getSelectorCallList() {
    return PsiTreeUtil.getChildrenOfTypeAsList(this, ThtrSelectorCall.class);
  }

}
