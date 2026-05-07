package com.github.alexpoliushkin.theater.thtrij.psi.impl;

import com.github.alexpoliushkin.theater.thtrij.psi.ThtrNamedElement;
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrScenarioDeclaration;
import com.github.alexpoliushkin.theater.thtrij.psi.ThtrTypes;
import com.intellij.extapi.psi.ASTWrapperPsiElement;
import com.intellij.lang.ASTNode;
import com.intellij.openapi.editor.Document;
import com.intellij.openapi.util.TextRange;
import com.intellij.psi.PsiDocumentManager;
import com.intellij.psi.PsiElement;
import com.intellij.psi.search.GlobalSearchScope;
import com.intellij.psi.search.LocalSearchScope;
import com.intellij.psi.search.SearchScope;
import com.intellij.psi.tree.TokenSet;
import com.intellij.util.IncorrectOperationException;
import org.jetbrains.annotations.NotNull;
import org.jetbrains.annotations.Nullable;

public abstract class ThtrNamedElementImpl extends ASTWrapperPsiElement implements ThtrNamedElement {
  private static final TokenSet NAME_TOKENS = TokenSet.create(
    ThtrTypes.IDENTIFIER,
    ThtrTypes.DOTTED_REF
  );

  public ThtrNamedElementImpl(@NotNull ASTNode node) {
    super(node);
  }

  @Override
  public @Nullable String getName() {
    PsiElement identifier = getNameIdentifier();
    return identifier == null ? null : identifier.getText();
  }

  @Override
  public PsiElement setName(@NotNull String name) throws IncorrectOperationException {
    if (this instanceof ThtrScenarioDeclaration && isRepoLibraryFile()) {
      throw new IncorrectOperationException("Repository library .thtr scenarios cannot be renamed from local usages");
    }

    PsiElement identifier = getNameIdentifier();
    if (identifier == null) {
      return this;
    }

    PsiDocumentManager documentManager = PsiDocumentManager.getInstance(getProject());
    Document document = documentManager.getDocument(getContainingFile());
    if (document == null) {
      throw new IncorrectOperationException("Cannot rename .thtr symbol without a document");
    }

    TextRange range = identifier.getTextRange();
    document.replaceString(range.getStartOffset(), range.getEndOffset(), name);
    documentManager.commitDocument(document);
    return this;
  }

  @Override
  public @Nullable PsiElement getNameIdentifier() {
    PsiElement child = getFirstChild();
    while (child != null) {
      ASTNode node = child.getNode();
      if (node != null && NAME_TOKENS.contains(node.getElementType())) {
        return child;
      }
      child = child.getNextSibling();
    }
    return null;
  }

  @Override
  public int getTextOffset() {
    PsiElement identifier = getNameIdentifier();
    return identifier == null ? super.getTextOffset() : identifier.getTextRange().getStartOffset();
  }

  @Override
  public @NotNull SearchScope getUseScope() {
    if (this instanceof ThtrScenarioDeclaration && isRepoLibraryFile()) {
      return GlobalSearchScope.projectScope(getProject());
    }
    return new LocalSearchScope(getContainingFile());
  }

  private boolean isRepoLibraryFile() {
    return getContainingFile() != null &&
      getContainingFile().getVirtualFile() != null &&
      getContainingFile().getVirtualFile().getPath().contains("/theater/lib/");
  }
}
