// This is a generated file. Not intended for manual editing.
package com.github.alexpoliushkin.theater.thtrij.psi;

import com.intellij.psi.tree.IElementType;
import com.intellij.psi.PsiElement;
import com.intellij.lang.ASTNode;
import com.github.alexpoliushkin.theater.thtrij.ThtrElementType;
import com.github.alexpoliushkin.theater.thtrij.ThtrTokenType;
import com.github.alexpoliushkin.theater.thtrij.psi.impl.*;

public interface ThtrTypes {

  IElementType ACT_DECLARATION = new ThtrElementType("ACT_DECLARATION");
  IElementType AUTH_DECLARATION = new ThtrElementType("AUTH_DECLARATION");
  IElementType BACKEND_DECLARATION = new ThtrElementType("BACKEND_DECLARATION");
  IElementType BIND_STATEMENT = new ThtrElementType("BIND_STATEMENT");
  IElementType CALL_DECLARATION = new ThtrElementType("CALL_DECLARATION");
  IElementType CAPTURE_AUTH_STATEMENT = new ThtrElementType("CAPTURE_AUTH_STATEMENT");
  IElementType DECLARATION = new ThtrElementType("DECLARATION");
  IElementType DEPENDENCY_STATEMENT = new ThtrElementType("DEPENDENCY_STATEMENT");
  IElementType DESCRIPTOR_REF = new ThtrElementType("DESCRIPTOR_REF");
  IElementType DO_STATEMENT = new ThtrElementType("DO_STATEMENT");
  IElementType EVENTUALLY_STATEMENT = new ThtrElementType("EVENTUALLY_STATEMENT");
  IElementType EXPECT_STATEMENT = new ThtrElementType("EXPECT_STATEMENT");
  IElementType EXPORT_STATEMENT = new ThtrElementType("EXPORT_STATEMENT");
  IElementType GENERATOR_CALL = new ThtrElementType("GENERATOR_CALL");
  IElementType HTTP_BLOCK = new ThtrElementType("HTTP_BLOCK");
  IElementType IDENTITY_DECLARATION = new ThtrElementType("IDENTITY_DECLARATION");
  IElementType LIST_VALUE = new ThtrElementType("LIST_VALUE");
  IElementType LOG_STATEMENT = new ThtrElementType("LOG_STATEMENT");
  IElementType NAME_STATEMENT = new ThtrElementType("NAME_STATEMENT");
  IElementType OBJECT_VALUE = new ThtrElementType("OBJECT_VALUE");
  IElementType POOL_DECLARATION = new ThtrElementType("POOL_DECLARATION");
  IElementType PROP_STATEMENT = new ThtrElementType("PROP_STATEMENT");
  IElementType RECORD_DECLARATION = new ThtrElementType("RECORD_DECLARATION");
  IElementType SCENARIO_DECLARATION = new ThtrElementType("SCENARIO_DECLARATION");
  IElementType SELECTOR_CALL = new ThtrElementType("SELECTOR_CALL");
  IElementType SESSION_DECLARATION = new ThtrElementType("SESSION_DECLARATION");
  IElementType STAGE_DECLARATION = new ThtrElementType("STAGE_DECLARATION");
  IElementType STATE_BLOCK = new ThtrElementType("STATE_BLOCK");
  IElementType TRANSITION_STATEMENT = new ThtrElementType("TRANSITION_STATEMENT");

  IElementType ACT = new ThtrTokenType("act");
  IElementType ALL = new ThtrTokenType("all");
  IElementType AND = new ThtrTokenType("and");
  IElementType ARROW = new ThtrTokenType("->");
  IElementType ASSERT = new ThtrTokenType("assert");
  IElementType AUTH = new ThtrTokenType("auth");
  IElementType BACKEND = new ThtrTokenType("backend");
  IElementType BAD_CHARACTER = new ThtrTokenType("bad_character");
  IElementType BAD_INDENT = new ThtrTokenType("bad_indent");
  IElementType BANG = new ThtrTokenType("!");
  IElementType BETWEEN = new ThtrTokenType("between");
  IElementType BIND = new ThtrTokenType("bind");
  IElementType CALL = new ThtrTokenType("call");
  IElementType CAPTURE_AUTH = new ThtrTokenType("capture_auth");
  IElementType COLON = new ThtrTokenType(":");
  IElementType COMMA = new ThtrTokenType(",");
  IElementType CONTAINS = new ThtrTokenType("contains");
  IElementType DECODE = new ThtrTokenType("decode");
  IElementType DEPENDENCY = new ThtrTokenType("dependency");
  IElementType DO = new ThtrTokenType("do");
  IElementType DOLLAR_REF = new ThtrTokenType("dollar_ref");
  IElementType DOT = new ThtrTokenType(".");
  IElementType DOTTED_REF = new ThtrTokenType("dotted_ref");
  IElementType DURATION = new ThtrTokenType("duration");
  IElementType ENTRY = new ThtrTokenType("entry");
  IElementType EQEQ = new ThtrTokenType("==");
  IElementType EQUALS = new ThtrTokenType("=");
  IElementType EVENTUALLY = new ThtrTokenType("eventually");
  IElementType EVERY = new ThtrTokenType("every");
  IElementType EXPECT = new ThtrTokenType("expect");
  IElementType EXPORT = new ThtrTokenType("export");
  IElementType FALSE = new ThtrTokenType("false");
  IElementType FIELD = new ThtrTokenType("field");
  IElementType GENERATE_REF = new ThtrTokenType("generate_ref");
  IElementType GT = new ThtrTokenType(">");
  IElementType GTE = new ThtrTokenType(">=");
  IElementType HAS = new ThtrTokenType("has");
  IElementType HTTP = new ThtrTokenType("http");
  IElementType IDENTIFIER = new ThtrTokenType("identifier");
  IElementType IDENTITY = new ThtrTokenType("identity");
  IElementType IS = new ThtrTokenType("is");
  IElementType ITEM = new ThtrTokenType("item");
  IElementType ITEMS = new ThtrTokenType("items");
  IElementType KEY = new ThtrTokenType("key");
  IElementType LACKS = new ThtrTokenType("lacks");
  IElementType LINE_COMMENT = new ThtrTokenType("line_comment");
  IElementType LIST = new ThtrTokenType("list");
  IElementType LOG = new ThtrTokenType("log");
  IElementType LT = new ThtrTokenType("<");
  IElementType LTE = new ThtrTokenType("<=");
  IElementType L_BRACE = new ThtrTokenType("{");
  IElementType L_BRACKET = new ThtrTokenType("[");
  IElementType L_PAREN = new ThtrTokenType("(");
  IElementType MATCHES = new ThtrTokenType("matches");
  IElementType NAME = new ThtrTokenType("name");
  IElementType NO = new ThtrTokenType("no");
  IElementType NULL = new ThtrTokenType("null");
  IElementType NUMBER = new ThtrTokenType("number");
  IElementType OBJECT = new ThtrTokenType("object");
  IElementType ON = new ThtrTokenType("on");
  IElementType PATH = new ThtrTokenType("path");
  IElementType PICK = new ThtrTokenType("pick");
  IElementType PIPE = new ThtrTokenType("|");
  IElementType POOL = new ThtrTokenType("pool");
  IElementType PROP = new ThtrTokenType("prop");
  IElementType RECORD = new ThtrTokenType("record");
  IElementType REGEXP = new ThtrTokenType("regexp");
  IElementType REPEATABLE = new ThtrTokenType("repeatable");
  IElementType R_BRACE = new ThtrTokenType("}");
  IElementType R_BRACKET = new ThtrTokenType("]");
  IElementType R_PAREN = new ThtrTokenType(")");
  IElementType SCENARIO = new ThtrTokenType("scenario");
  IElementType SESSION = new ThtrTokenType("session");
  IElementType STAGE = new ThtrTokenType("stage");
  IElementType STATE = new ThtrTokenType("state");
  IElementType STRING = new ThtrTokenType("string");
  IElementType TRUE = new ThtrTokenType("true");
  IElementType WHEN = new ThtrTokenType("when");
  IElementType WHERE = new ThtrTokenType("where");

  class Factory {
    public static PsiElement createElement(ASTNode node) {
      IElementType type = node.getElementType();
      if (type == ACT_DECLARATION) {
        return new ThtrActDeclarationImpl(node);
      }
      else if (type == AUTH_DECLARATION) {
        return new ThtrAuthDeclarationImpl(node);
      }
      else if (type == BACKEND_DECLARATION) {
        return new ThtrBackendDeclarationImpl(node);
      }
      else if (type == BIND_STATEMENT) {
        return new ThtrBindStatementImpl(node);
      }
      else if (type == CALL_DECLARATION) {
        return new ThtrCallDeclarationImpl(node);
      }
      else if (type == CAPTURE_AUTH_STATEMENT) {
        return new ThtrCaptureAuthStatementImpl(node);
      }
      else if (type == DECLARATION) {
        return new ThtrDeclarationImpl(node);
      }
      else if (type == DEPENDENCY_STATEMENT) {
        return new ThtrDependencyStatementImpl(node);
      }
      else if (type == DESCRIPTOR_REF) {
        return new ThtrDescriptorRefImpl(node);
      }
      else if (type == DO_STATEMENT) {
        return new ThtrDoStatementImpl(node);
      }
      else if (type == EVENTUALLY_STATEMENT) {
        return new ThtrEventuallyStatementImpl(node);
      }
      else if (type == EXPECT_STATEMENT) {
        return new ThtrExpectStatementImpl(node);
      }
      else if (type == EXPORT_STATEMENT) {
        return new ThtrExportStatementImpl(node);
      }
      else if (type == GENERATOR_CALL) {
        return new ThtrGeneratorCallImpl(node);
      }
      else if (type == HTTP_BLOCK) {
        return new ThtrHttpBlockImpl(node);
      }
      else if (type == IDENTITY_DECLARATION) {
        return new ThtrIdentityDeclarationImpl(node);
      }
      else if (type == LIST_VALUE) {
        return new ThtrListValueImpl(node);
      }
      else if (type == LOG_STATEMENT) {
        return new ThtrLogStatementImpl(node);
      }
      else if (type == NAME_STATEMENT) {
        return new ThtrNameStatementImpl(node);
      }
      else if (type == OBJECT_VALUE) {
        return new ThtrObjectValueImpl(node);
      }
      else if (type == POOL_DECLARATION) {
        return new ThtrPoolDeclarationImpl(node);
      }
      else if (type == PROP_STATEMENT) {
        return new ThtrPropStatementImpl(node);
      }
      else if (type == RECORD_DECLARATION) {
        return new ThtrRecordDeclarationImpl(node);
      }
      else if (type == SCENARIO_DECLARATION) {
        return new ThtrScenarioDeclarationImpl(node);
      }
      else if (type == SELECTOR_CALL) {
        return new ThtrSelectorCallImpl(node);
      }
      else if (type == SESSION_DECLARATION) {
        return new ThtrSessionDeclarationImpl(node);
      }
      else if (type == STAGE_DECLARATION) {
        return new ThtrStageDeclarationImpl(node);
      }
      else if (type == STATE_BLOCK) {
        return new ThtrStateBlockImpl(node);
      }
      else if (type == TRANSITION_STATEMENT) {
        return new ThtrTransitionStatementImpl(node);
      }
      throw new AssertionError("Unknown element type: " + type);
    }
  }
}
