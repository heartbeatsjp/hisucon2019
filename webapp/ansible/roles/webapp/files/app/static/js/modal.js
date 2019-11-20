$(function () {
    $('#open-modal').click(function(){
        $('#modal-back').fadeIn();
    });
    $('#close-modal, #cancel-btn').click(function(){
        $('#modal-back').fadeOut();
    });
});
