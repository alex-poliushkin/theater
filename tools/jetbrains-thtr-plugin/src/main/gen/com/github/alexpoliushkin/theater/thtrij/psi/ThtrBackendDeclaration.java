// This is a generated file. Not intended for manual editing.
package com.github.alexpoliushkin.theater.thtrij.psi;

import java.util.List;
import org.jetbrains.annotations.*;
import com.intellij.psi.PsiElement;

public interface ThtrBackendDeclaration extends PsiElement {

  @NotNull
  List<ThtrDescriptorRef> getDescriptorRefList();

  @NotNull
  List<ThtrGeneratorCall> getGeneratorCallList();

  @NotNull
  List<ThtrListValue> getListValueList();

  @NotNull
  List<ThtrObjectValue> getObjectValueList();

  @NotNull
  List<ThtrSelectorCall> getSelectorCallList();

}
