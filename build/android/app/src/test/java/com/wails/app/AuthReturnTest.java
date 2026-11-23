package com.wails.app;

import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertTrue;

import org.junit.Test;

/** 認可の完了ページの「戻る」リンクからの Intent の判定（AuthReturn）のテスト。 */
public class AuthReturnTest {

    private static final String VIEW = "android.intent.action.VIEW";

    @Test
    public void matchesOnlyTheReturnLink() {
        assertTrue(AuthReturn.matches(VIEW, "jp.ousias.hitomado", "auth-return"));
    }

    @Test
    public void rejectsOtherActionSchemeOrHost() {
        assertFalse(AuthReturn.matches("android.intent.action.MAIN", "jp.ousias.hitomado", "auth-return"));
        assertFalse(AuthReturn.matches(null, "jp.ousias.hitomado", "auth-return"));
        assertFalse(AuthReturn.matches(VIEW, "https", "auth-return"));
        assertFalse(AuthReturn.matches(VIEW, "jp.ousias.hitomado", "other"));
        assertFalse(AuthReturn.matches(VIEW, null, null));
    }

    @Test
    public void nullIntentIsNotTheReturnLink() {
        assertFalse(AuthReturn.matches(null));
    }

    /** Go の callbackpage.AndroidReturnURL と、AndroidManifest.xml の intent-filter が同じ scheme・host を持つ（Go 側のテストが manifest と照合する）。 */
    @Test
    public void constantsFormTheReturnUrl() {
        assertTrue("jp.ousias.hitomado://auth-return".equals(AuthReturn.SCHEME + "://" + AuthReturn.HOST));
    }
}
