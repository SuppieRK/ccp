def test_pass():
    value = 2 + 2
    assert value == 4


def test_fail():
    print("captured stdout call")
    left = {"ok": False}
    right = {"ok": True}
    assert left == right
